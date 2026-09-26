package remote

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ioncode/go_short/internal/router/audit"
	"go.uber.org/zap"
)

// mockTransport имитирует моментальный успешный сетевой ответ 200 OK
// без реального создания сетевых соединений.
type mockTransportB struct{}

func (m *mockTransportB) RoundTrip(req *http.Request) (*http.Response, error) {
	// Возвращаем пустой успешный ответ. Тело закрывается автоматически.
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
	}, nil
}

// BenchmarkRemoteObserver_SingleThread замеряет скорость работы приёмника
// при последовательном вызове в одном потоке.
func BenchmarkRemoteObserver_SingleThread(b *testing.B) {
	logger := zap.NewNop()
	observer, err := NewRemoteObserver("http://mock-target.local", logger)
	if err != nil {
		b.Fatalf("не удалось инициализировать обсервер: %v", err)
	}

	// Подменяем реальный транспорт на наш мок через добавленный метод
	observer.client.SetTransport(&mockTransportB{})

	event := audit.Event{
		TS:         time.Now().Unix(),
		UserID:     "benchmark-user-999",
		Action:     "shorten",
		URL:        "https://some-very-long-and-complex-url-to-stress-test-json-marshaler.com",
		Method:     "POST",
		Path:       "/api/shorten",
		StatusCode: 201,
	}

	ctx := context.Background()
	b.ResetTimer()

	for b.Loop() {
		observer.OnRequest(ctx, event)
	}
}

// BenchmarkRemoteObserver_Parallel замеряет производительность приёмника
// при одновременной работе из сотен параллельных горутин.
func BenchmarkRemoteObserver_Parallel(b *testing.B) {
	logger := zap.NewNop()
	observer, err := NewRemoteObserver("http://mock-target.local", logger)
	if err != nil {
		b.Fatalf("не удалось инициализировать обсервер: %v", err)
	}

	// Подменяем реальный транспорт на наш мок
	observer.client.SetTransport(&mockTransportB{})

	event := audit.Event{
		TS:         time.Now().Unix(),
		UserID:     "benchmark-user-999",
		Action:     "follow",
		URL:        "https://example.com",
		Method:     "GET",
		Path:       "/alias",
		StatusCode: 307,
	}

	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			observer.OnRequest(ctx, event)
		}
	})
}
