package remote

import (
	"context"
	"net/url"

	json "github.com/goccy/go-json"
	"github.com/ioncode/go_short/pkg/http/audit"

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

	cb := NewCircuitBreaker()
	client := NewHTTPClient(targetURL)
	client.backoffFn = cb.CalculateBackoff

	return &RemoteObserver{
		client: client,
		cb:     cb,
		logger: logger.Named("audit_remote_observer"),
	}, nil
}

// Name возвращает человекочитаемое имя сетевого приемника логов.
func (r *RemoteObserver) Name() string { return "Сетевой приемник" }

// OnRequest маршалит событие через быстрый json.Marshal и отправляет его в сеть.
func (r *RemoteObserver) OnRequest(ctx context.Context, e audit.Event) {
	if !r.cb.AllowRequest() {
		r.logger.Warn("Предохранитель РАЗОМКНУТ. Сетевой запрос пропущен.")
		return
	}
	defer r.cb.ReleaseHalfOpenSlot()

	payload, err := json.Marshal(e)
	if err != nil {
		r.logger.Error("Не удалось закодировать событие для отправки", zap.Error(err))
		return
	}

	retries := r.cb.MaxRetries()
	if r.cb.IsHalfOpen() {
		retries = 1
	}

	sendErr := r.client.SendPostRequest(ctx, payload, retries)

	if sendErr == nil {
		r.cb.RecordSuccess(r.logger)
	} else {
		r.logger.Error("Сетевая отправка логов завершилась сбоем после всех попыток. Фиксируем ошибку в предохранителе.", zap.Error(sendErr))
		r.cb.RecordFailure(r.logger)
	}
}

func (r *RemoteObserver) Close() error {
	r.client.CloseIdleConnections()
	return nil
}
