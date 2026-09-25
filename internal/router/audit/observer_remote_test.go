package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"go.uber.org/zap"
)

// TestRemoteObserver_Success проверяет успешный сценарий отправки лога.
func TestRemoteObserver_Success(t *testing.T) {
	// 1. Создаем тестовое событие аудита
	event := Event{
		TS:         time.Now().Unix(),
		UserID:     "user-123",
		Action:     "shorten",
		URL:        "https://example.com",
		Method:     "POST",
		Path:       "/api/shorten",
		StatusCode: 201,
	}

	var receivedRequest bool

	// 2. Запускаем локальный мок-сервер
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = true

		// Проверяем заголовки запроса
		if r.Method != http.MethodPost {
			t.Errorf("Ожидался метод POST, получен: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Ожидался Content-Type application/json, получен: %s", r.Header.Get("Content-Type"))
		}

		// Декодируем и валидируем тело запроса
		var receivedEvent Event
		if err := json.NewDecoder(r.Body).Decode(&receivedEvent); err != nil {
			t.Fatalf("Не удалось декодировать тело запроса: %v", err)
		}

		if receivedEvent.UserID != event.UserID || receivedEvent.URL != event.URL {
			t.Errorf("Полученные данные не совпадают с отправленными. Отправлено: %+v, Получено: %+v", event, receivedEvent)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 3. Инициализируем RemoteObserver и отправляем запрос
	logger := zap.NewNop() // Выключаем вывод логов в консоль во время тестов
	observer := NewRemoteObserver(server.URL, logger)

	observer.OnRequest(context.Background(), event)

	// 4. Проверяем финальное состояние
	if !receivedRequest {
		t.Error("Сервер-приемник так и не получил запрос от RemoteObserver")
	}
	if observer.circuitState != StateClosed {
		t.Errorf("Ожидалось состояние предохранителя CLOSED, получено: %s", observer.circuitState)
	}
}

// TestRemoteObserver_CircuitBreaker_Trips проверяет, что предохранитель
// корректно размыкает цепь (переходит в состояние OPEN) после достижения лимита ошибок.
func TestRemoteObserver_CircuitBreaker_Trips(t *testing.T) {
	var totalRequests int32

	// 1. Запускаем сервер, который всегда отвечает ошибкой 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&totalRequests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := zap.NewNop()
	observer := NewRemoteObserver(server.URL, logger)

	event := Event{Action: "test-failure"}

	// 2. Отправляем ровно failureThreshold (5) запросов, чтобы заставить предохранитель сработать.
	// Каждый OnRequest выполнит по 3 ретрая (maxRetries) внутри себя перед фиксацией сбоя.
	for i := 0; i < failureThreshold; i++ {
		observer.OnRequest(context.Background(), event)
	}

	// Проверяем, что состояние изменилось на OPEN
	if observer.circuitState != StateOpen {
		t.Errorf("Ожидалось, что предохранитель перейдет в состояние OPEN, текущее состояние: %s", observer.circuitState)
	}

	// Считаем общее количество попыток: 5 вызовов OnRequest * 3 ретрая = 15 запросов в сеть
	expectedRequests := int32(failureThreshold * maxRetries)
	if atomic.LoadInt32(&totalRequests) != expectedRequests {
		t.Errorf("Ожидалось %d сетевых запросов до размыкания цепи, сервер зафиксировал: %d", expectedRequests, totalRequests)
	}

	// 3. Отправляем шестой запрос, когда предохранитель УЖЕ в состоянии OPEN
	observer.OnRequest(context.Background(), event)

	// Проверяем, что запрос был мгновенно заблокирован предохранителем и счетчик на сервере НЕ увеличился
	if atomic.LoadInt32(&totalRequests) != expectedRequests {
		t.Errorf("Предохранитель пропустил запрос в состоянии OPEN! Количество запросов на сервере выросло до: %d", totalRequests)
	}
}

// mockTransport имитирует моментальный успешный сетевой ответ 200 OK
// без реального создания сетевых соединений.
type mockTransport struct{}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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
	observer := NewRemoteObserver("http://mock-target.local", logger)

	// Подменяем реальный транспорт на наш мок
	observer.client.Transport = &mockTransport{}

	event := Event{
		TS:         time.Now().Unix(),
		UserID:     "benchmark-user-999",
		Action:     "shorten",
		URL:        "https://some-very-long-and-complex-url-to-stress-test-json-marshaler.com",
		Method:     "POST",
		Path:       "/api/shorten",
		StatusCode: 201,
	}

	ctx := context.Background()

	for b.Loop() {
		observer.OnRequest(ctx, event)
	}
}

// BenchmarkRemoteObserver_Parallel замеряет производительность приёмника
// при одновременной работе из сотен параллельных горутин.
// Этот тест показывает, насколько эффективно мьютексы предохранителя справляются с нагрузкой.
func BenchmarkRemoteObserver_Parallel(b *testing.B) {
	logger := zap.NewNop()
	observer := NewRemoteObserver("http://mock-target.local", logger)
	observer.client.Transport = &mockTransport{}

	event := Event{
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
