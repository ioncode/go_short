package audit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"go.uber.org/zap"
)

// CircuitState определяет тип для состояний предохранителя (Circuit Breaker).
type CircuitState string

const (
	// StateClosed указывает, что цепь замкнута, система работает штатно,
	// и все запросы отправляются во внешнюю сеть.
	StateClosed CircuitState = "CLOSED"

	// StateOpen указывает, что цепь разомкнута из-за превышения лимита ошибок.
	// Сетевые запросы мгновенно блокируются для экономии ресурсов.
	StateOpen CircuitState = "OPEN"

	// StateHalfOpen указывает, что период остывания предохранителя завершен.
	// Система пропускает один тестовый запрос для проверки доступности сервера.
	StateHalfOpen CircuitState = "HALF-OPEN"
)

const (
	maxRetries       = 3
	baseBackoff      = 200 * time.Millisecond
	maxBackoff       = 2 * time.Second
	failureThreshold = 5
	cooldownTimeout  = 30 * time.Second
)

// RemoteObserver реализует интерфейс Observer для асинхронной
// отправки событий аудита на удаленный server по протоколу HTTP POST.
type RemoteObserver struct {
	client *http.Client
	url    string
	logger *zap.Logger

	// Предохранитель
	cbMu             sync.RWMutex
	failureCount     int
	circuitState     CircuitState
	nextAttemptAfter time.Time

	// Пул предопределенных HTTP-запросов для исключения аллокаций сетевого стека
	reqPool sync.Pool
}

// NewRemoteObserver инициализирует и возвращает новый экземпляр RemoteObserver.
func NewRemoteObserver(targetURL string, logger *zap.Logger) *RemoteObserver {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		logger.Error("Критическая ошибка: не удалось распарсить URL удаленного аудита", zap.Error(err))
	}

	obs := &RemoteObserver{
		url:          targetURL,
		client:       &http.Client{Timeout: 3 * time.Second},
		logger:       logger.Named("audit_remote_observer"),
		circuitState: StateClosed,
	}

	// Инициализируем пул запросов. Карты заголовков и URL парсятся ОДИН раз при старте приложения.
	obs.reqPool = sync.Pool{
		New: func() any {
			req := &http.Request{
				Method:     http.MethodPost,
				URL:        parsedURL,
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
				Host:       parsedURL.Host,
			}
			req.Header.Set("Content-Type", "application/json")
			return req
		},
	}

	return obs
}

// Name возвращает человекочитаемое имя сетевого приемника логов.
func (r *RemoteObserver) Name() string { return "Сетевой приемник" }

// OnRequest сериализует пришедшее событие Event в JSON и отправляет его
// на удаленный сервер с минимальным количеством аллокаций памяти.
func (r *RemoteObserver) OnRequest(ctx context.Context, e Event) {
	if !r.allowRequest() {
		r.logger.Warn("Предохранитель РАЗОМКНУТ. Сетевой запрос пропущен для экономии ресурсов воркеров.")
		return
	}

	// Использование json.Marshal поверх структуры со скрытыми тегами
	payload, err := json.Marshal(e)
	if err != nil {
		r.logger.Error("Не удалось закодировать событие для отправки", zap.Error(err))
		return
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return
		}

		err := r.sendRequest(ctx, payload)
		if err == nil {
			r.recordSuccess()
			return
		}

		if attempt == maxRetries-1 {
			r.logger.Error("Превышено число попыток. Фиксируем сбой для предохранителя.")
			r.recordFailure()
			return
		}

		backoff := baseBackoff * (1 << attempt)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		jitter := time.Duration(rand.Intn(int(backoff) / 5))
		sleepDuration := backoff + jitter

		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return
		}
	}
}

// allowRequest проверяет состояние предохранителя атомарно.
func (r *RemoteObserver) allowRequest() bool {
	r.cbMu.Lock()
	defer r.cbMu.Unlock()

	if r.circuitState == StateClosed {
		return true
	}

	if r.circuitState == StateOpen {
		if time.Now().After(r.nextAttemptAfter) {
			r.circuitState = StateHalfOpen
			r.logger.Info("Предохранитель перешел в режим ожидания (HALF-OPEN). Пробуем отправить тестовый лог.")
			return true
		}
		return false
	}

	return false
}

// recordSuccess сбрасывает счетчики ошибок.
func (r *RemoteObserver) recordSuccess() {
	r.cbMu.Lock()
	defer r.cbMu.Unlock()

	if r.circuitState == StateHalfOpen {
		r.logger.Info("Тестовый запрос успешен! Предохранитель ВОССТАНОВЛЕН (CLOSED).")
	}
	r.circuitState = StateClosed
	r.failureCount = 0
}

// recordFailure фиксирует сбой.
func (r *RemoteObserver) recordFailure() {
	r.cbMu.Lock()
	defer r.cbMu.Unlock()

	r.failureCount++

	if r.circuitState == StateHalfOpen || r.failureCount >= failureThreshold {
		r.circuitState = StateOpen
		r.nextAttemptAfter = time.Now().Add(cooldownTimeout)
		r.logger.Error("Предохранитель СРАБОТАЛ (OPEN). Сетевой аудит временно отключен.",
			zap.Duration("cooldown", cooldownTimeout),
			zap.Int("total_failures", r.failureCount),
		)
	}
}

// sendRequest выполняет сетевую операцию, переиспользуя объект http.Request из пула.
func (r *RemoteObserver) sendRequest(ctx context.Context, payload []byte) error {
	// Достаем готовый предскомпилированный скелет запроса
	req := r.reqPool.Get().(*http.Request)

	// Возвращаем запрос назад в пул сразу по окончании работы метода
	defer r.reqPool.Put(req)

	// Перепривязываем текущий контекст горутины
	req = req.WithContext(ctx)

	// Подменяем тело запроса "на лету" без перевыделения структуры.
	// bytes.NewReader работает поверх существующего payload без копирования.
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("bad status code")
	}
	return nil
}

// Close закрывает свободные сетевые соединения.
func (r *RemoteObserver) Close() error {
	r.client.CloseIdleConnections()
	return nil
}
