package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// HTTPClient инкапсулирует стандартный http.Client и предоставляет механизмы
// повторных попыток с exponential backoff и переиспользования буферов.
type HTTPClient struct {
	client     *http.Client
	url        string
	bufPool    sync.Pool
	maxRetries int
	backoffFn  func(attempt int) time.Duration // Динамическая функция расчета бэк-оффа
}

// NewHTTPClient инициализирует клиент для работы с указанным целевым URL и настройками ретраев.
func NewHTTPClient(targetURL string, maxRetries int) *HTTPClient {
	return &HTTPClient{
		url:        targetURL,
		client:     &http.Client{Timeout: 3 * time.Second},
		maxRetries: maxRetries,
		bufPool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, 256))
			},
		},
	}
}

// SendPostRequest выполняет отправку HTTP POST-запроса с логикой повторных попыток.
// Если isTestRequest равно true (режим HALF-OPEN в Circuit Breaker), выполняется только 1 попытка.
func (c *HTTPClient) SendPostRequest(ctx context.Context, payload []byte, isTestRequest bool) error {
	buf := c.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(payload)

	defer c.bufPool.Put(buf)

	retries := c.maxRetries
	if isTestRequest {
		retries = 1 // В режиме HALF-OPEN запрещены повторные попытки
	}

	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = c.doSingleRequest(ctx, buf, payload)
		if lastErr == nil {
			return nil // Успешная отправка
		}

		// Если это не последняя попытка, делаем backoff паузу
		if attempt < retries-1 {
			sleepDuration := c.backoffFn(attempt)
			timer := time.NewTimer(sleepDuration)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
			timer.Stop()
		}
	}

	return lastErr
}

// doSingleRequest выполняет одну физическую HTTP-отправку.
func (c *HTTPClient) doSingleRequest(ctx context.Context, buf *bytes.Buffer, payload []byte) error {
	// Сбрасываем буфер к началу перед повторным чтением
	buf.Reset()
	buf.Write(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	// Вычитываем и закрываем тело для сохранения Keep-Alive сессии
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("bad status code")
	}
	return nil
}

// CloseIdleConnections инициирует принудительное закрытие всех простаивающих соединений.
func (c *HTTPClient) CloseIdleConnections() {
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
}

// SetTransport позволяет подменить сетевой транспорт (например, на мок в бенчмарках).
func (c *HTTPClient) SetTransport(transport http.RoundTripper) {
	if c.client != nil {
		c.client.Transport = transport
	}
}
