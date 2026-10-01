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
// переиспользования оперативной памяти для минимизации нагрузки на Garbage Collector.
type HTTPClient struct {
	client  *http.Client
	url     string
	bufPool sync.Pool
}

// NewHTTPClient инициализирует клиент для работы с указанным целевым URL.
// Настраивает пул буферов со стартовой емкостью среза в 256 байт под размер 4 полей.
func NewHTTPClient(targetURL string) *HTTPClient {
	return &HTTPClient{
		url:    targetURL,
		client: &http.Client{Timeout: 3 * time.Second},
		bufPool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, 256))
			},
		},
	}
}

// SendPostRequest выполняет отправку HTTP POST-запроса на удаленный сервер.
// Метод копирует входящий payload в буфер из пула, изолируя данные от гонок памяти.
func (c *HTTPClient) SendPostRequest(ctx context.Context, payload []byte) error {
	buf := c.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, buf)
	if err != nil {
		c.bufPool.Put(buf)
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		c.bufPool.Put(buf)
		return err
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		c.bufPool.Put(buf) // Возврат буфера строго после завершения чтения сетевым стеком
	}()

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
