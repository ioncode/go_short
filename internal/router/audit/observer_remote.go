package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"time"

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
// отправки событий аудита на удаленный сервер по протоколу HTTP POST.
//
// Обладает встроенной отказоустойчивостью: использует экспоненциальную
// задержку с рандомизацией (Exponential Backoff с Jitter) для повторных попыток
// и паттерн Circuit Breaker (Предохранитель) для предотвращения лавинных сбоев.
type RemoteObserver struct {
	client *http.Client
	url    string
	logger *zap.Logger

	cbMu             sync.RWMutex
	failureCount     int
	circuitState     CircuitState
	nextAttemptAfter time.Time
}

// NewRemoteObserver инициализирует и возвращает новый экземпляр RemoteObserver.
// Принимает целевой URL сервера-приемника логов и настроенный логгер zap.Logger.
func NewRemoteObserver(url string, logger *zap.Logger) *RemoteObserver {
	return &RemoteObserver{
		url:          url,
		client:       &http.Client{Timeout: 3 * time.Second},
		logger:       logger.Named("audit_remote_observer"),
		circuitState: StateClosed,
	}
}

// Name возвращает человекочитаемое имя сетевого приемника логов.
// Используется диспетчером Auditor для ведения системных журналов остановки и регистрации.
func (r *RemoteObserver) Name() string { return "Сетевой приемник" }

// OnRequest сериализует пришедшее событие Event в JSON и отправляет его
// на удаленный сервер. Метод вызывается асинхронно пулом фоновых воркеров Auditor.
// В случае сетевых сбоев метод автоматически запускает цепочку ретраев.
func (r *RemoteObserver) OnRequest(ctx context.Context, e Event) {
	if !r.allowRequest() {
		r.logger.Warn("Предохранитель РАЗОМКНУТ. Сетевой запрос пропущен для экономии ресурсов воркеров.")
		return
	}

	// Использование json.Marshal сразу создает изолированный срез байт в памяти.
	// Это исключает гонку данных с другими воркерами и не требует сложного пулирования.
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

// allowRequest проверяет состояние предохранителя и решает,
// разрешено ли выполнять физический сетевой запрос в данный момент времени.
func (r *RemoteObserver) allowRequest() bool {
	r.cbMu.Lock()
	defer r.cbMu.Unlock()

	if r.circuitState == StateClosed {
		return true
	}

	if r.circuitState == StateOpen {
		// Если время остывания прошло, ТОЛЬКО один поток переводит в HALF-OPEN и идет в сеть
		if time.Now().After(r.nextAttemptAfter) {
			r.circuitState = StateHalfOpen
			r.logger.Info("Предохранитель перешел в режим ожидания (HALF-OPEN). Пробуем отправить тестовый лог.")
			return true
		}
		return false
	}

	// Если состояние УЖЕ HALF-OPEN (тестовый запрос уже летит в сеть),
	// все остальные параллельные потоки на это время блокируются.
	return false
}

// recordSuccess сбрасывает счетчики ошибок и переводит предохранитель
// в исходное рабочее состояние StateClosed после успешной сетевой операции.
func (r *RemoteObserver) recordSuccess() {
	r.cbMu.Lock()
	defer r.cbMu.Unlock()

	if r.circuitState == StateHalfOpen {
		r.logger.Info("Тестовый запрос успешен! Предохранитель ВОССТАНОВЛЕН (CLOSED).")
	}
	r.circuitState = StateClosed
	r.failureCount = 0
}

// recordFailure увеличивает счетчик сбоев и размыкает цепь предохранителя (StateOpen),
// если лимит ошибок исчерпан или если тестовый запрос в режиме HALF-OPEN завершился неудачей.
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

// sendRequest выполняет единичный атомарный HTTP POST запрос с передачей сырых JSON-байтов.
func (r *RemoteObserver) sendRequest(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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

// Close закрывает все неиспользуемые праздные (idle) постоянные сетевые соединения,
// удерживаемые внутренним http.Client. Вызывается автоматически при graceful shutdown аудитора.
func (r *RemoteObserver) Close() error {
	r.client.CloseIdleConnections()
	return nil
}
