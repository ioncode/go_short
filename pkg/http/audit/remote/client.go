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
	client    *http.Client
	url       string
	bufPool   sync.Pool
	backoffFn func(attempt int) time.Duration // Динамическая функция расчета бэк-оффа
}

// NewHTTPClient инициализирует клиент для работы с указанным целевым URL.
// Задает безопасную дефолтную функцию бэк-оффа для предотвращения паник при ретраях.
func NewHTTPClient(targetURL string) *HTTPClient {
	return &HTTPClient{
		url:    targetURL,
		client: &http.Client{Timeout: 3 * time.Second},
		bufPool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, 256))
			},
		},
		// ГАРАНТИЯ БЕЗОПАСНОСТИ: инициализируем дефолтный линейно-экспоненциальный backoff.
		// Если поле не переопределят снаружи, рантайм больше никогда не упадет по nil-указателю.
		backoffFn: func(attempt int) time.Duration {
			return time.Duration(1+attempt*2) * time.Second
		},
	}
}

// SendPostRequest выполняет отправку HTTP POST-запроса с логикой повторных попыток.
func (c *HTTPClient) SendPostRequest(ctx context.Context, payload []byte, retries int) error {
	if retries < 1 {
		retries = 1
	}

	buf := c.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(payload)
	defer c.bufPool.Put(buf)

	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = c.doSingleRequest(ctx, buf, payload)
		if lastErr == nil {
			return nil
		}

		// Защита внутри цикла select теперь подкреплена гарантированным наличием backoffFn
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
