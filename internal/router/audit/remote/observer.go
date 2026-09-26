package remote

import (
	"context"
	"net/url"
	"time"

	json "github.com/goccy/go-json"
	"github.com/ioncode/go_short/internal/router/audit"
	"go.uber.org/zap"
)

// RemoteObserver координирует работу предохранителя и HTTP-клиента.
type RemoteObserver struct {
	client *HTTPClient
	cb     *CircuitBreaker
	logger *zap.Logger
}

// NewRemoteObserver инициализирует и возвращает новый оркестратор сетевого аудита.
func NewRemoteObserver(targetURL string, logger *zap.Logger) (*RemoteObserver, error) {
	_, err := url.ParseRequestURI(targetURL)
	if err != nil {
		logger.Error("Критическая ошибка: не удалось распарсить URL удаленного аудита", zap.Error(err))
		return nil, err
	}

	return &RemoteObserver{
		client: NewHTTPClient(targetURL),
		cb:     NewCircuitBreaker(),
		logger: logger.Named("audit_remote_observer"),
	}, nil
}

// Name возвращает человекочитаемое имя сетевого приемника логов.
func (r *RemoteObserver) Name() string { return "Сетевой приемник" }

// OnRequest маршалит событие через быстрый json.Marshal и отправляет его в сеть.
func (r *RemoteObserver) OnRequest(ctx context.Context, e audit.Event) {
	// 1. Проверяем состояние предохранителя
	if !r.cb.AllowRequest() {
		r.logger.Warn("Предохранитель РАЗОМКНУТ. Сетевой запрос пропущен.")
		return
	}
	// Гарантируем очистку слота HALF-OPEN при любом аварийном выходе (в т.ч. отмене контекста)
	defer r.cb.ReleaseHalfOpenSlot()

	// 2. Быстрый маршалинг лога
	payload, err := json.Marshal(e)
	if err != nil {
		r.logger.Error("Не удалось закодировать событие для отправки", zap.Error(err))
		return
	}

	// Запоминаем, является ли текущий запрос тестовым
	isTestRequest := r.cb.IsHalfOpen()

	maxRetries := r.cb.MaxRetries()
	var sendErr error

	// 3. Цикл отправки и повторных попыток
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return
		}

		sendErr = r.client.SendPostRequest(ctx, payload)
		if sendErr == nil {
			r.cb.RecordSuccess(r.logger)
			return
		}

		// В состоянии HALF-OPEN запрещены повторные попытки (ретраи). Сбой переводит цепь в OPEN.
		if isTestRequest {
			break
		}

		// Ретраи отрабатывают только для штатного режима CLOSED
		if attempt < maxRetries-1 {
			sleepDuration := r.cb.CalculateBackoff(attempt)
			select {
			case <-time.After(sleepDuration):
			case <-ctx.Done():
				return
			}
		}
	}

	// Фиксируем финальный сбой в стейт-машине, если запрос окончательно провалился
	if sendErr != nil {
		r.logger.Error("Сетевая отправка логов завершилась сбоем. Фиксируем ошибку в предохранителе.", zap.Error(sendErr))
		r.cb.RecordFailure(r.logger)
	}
}

// Close закрывает свободные сетевые соединения клиента.
func (r *RemoteObserver) Close() error {
	r.client.CloseIdleConnections()
	return nil
}
