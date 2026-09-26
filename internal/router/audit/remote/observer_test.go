package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	json "github.com/goccy/go-json"
	"github.com/ioncode/go_short/internal/router/audit"
	"go.uber.org/zap"
)

// mockTransport имитирует моментальный сетевой ответ и валидирует
// побайтовое содержимое отправляемого JSON-payload.
type mockTransport struct {
	t             *testing.T
	roundTripFunc func(req *http.Request) (*http.Response, error)
	calls         atomic.Int32
	expectedEvent audit.Event
}

// RoundTrip реализует стандартный интерфейс http.RoundTripper.
// Метод атомарно считает вызовы и проверяет структуру входящего JSON.
func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.calls.Add(1)

	// 1. Проверяем заголовки протокола
	if contentType := req.Header.Get("Content-Type"); contentType != "application/json" {
		m.t.Errorf("Ожидался Content-Type 'application/json', получено: '%s'", contentType)
	}

	// 2. Вычитываем тело запроса
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		m.t.Errorf("Не удалось прочитать тело запроса: %v", err)
	}
	_ = req.Body.Close()

	// 3. Декодируем сырой JSON в карту для проверки скрытых полей.
	// Нам нужно убедиться, что поля с тегом json:"-" физически отсутствуют в строке.
	var rawMap map[string]any
	if err := json.Unmarshal(bodyBytes, &rawMap); err != nil {
		m.t.Errorf("Удаленный сервер не смог распарсить входящий JSON: %v. Payload: %s", err, string(bodyBytes))
	}

	// Проверяем работу тегов json:"-"
	ignoredFields := []string{"Method", "Path", "StatusCode", "CustomData", "method", "path", "status_code", "custom_data"}
	for _, field := range ignoredFields {
		if _, exists := rawMap[field]; exists {
			m.t.Errorf("Критическая уязвимость: скрытое поле '%s' улетело в сеть, нарушая тег json:\"-\"", field)
		}
	}

	// 4. Декодируем в целевую структуру для сверки бизнес-данных
	var actualEvent audit.Event
	if err := json.Unmarshal(bodyBytes, &actualEvent); err != nil {
		m.t.Errorf("Ошибка десериализации события: %v", err)
	}

	if actualEvent.TS != m.expectedEvent.TS {
		m.t.Errorf("Несовпадение TS. Ожидалось: %d, получено: %d", m.expectedEvent.TS, actualEvent.TS)
	}
	if actualEvent.Action != m.expectedEvent.Action {
		m.t.Errorf("Несовпадение Action. Ожидалось: %s, получено: %s", m.expectedEvent.Action, actualEvent.Action)
	}
	if actualEvent.URL != m.expectedEvent.URL {
		m.t.Errorf("Несовпадение URL. Ожидалось: %s, получено: %s", m.expectedEvent.URL, actualEvent.URL)
	}
	if actualEvent.UserID != m.expectedEvent.UserID {
		m.t.Errorf("Несовпадение UserID. Ожидалось: %s, получено: %s", m.expectedEvent.UserID, actualEvent.UserID)
	}

	// Вызываем кастомное поведение (успех или симуляция ошибки сети)
	return m.roundTripFunc(req)
}

// TestRemoteObserver_Lifecycle_Synctest сквозным образом тестирует всю стейт-машину
// предохранителя, логику сетевых задержек (Backoff) и побайтовое содержимое JSON
// в изолированном пространстве виртуального времени Go 1.26.1.
func TestRemoteObserver_Lifecycle_Synctest(t *testing.T) {
	logger := zap.NewNop()
	observer, err := NewRemoteObserver("http://mock-target.local", logger)
	if err != nil {
		t.Fatalf("не удалось создать наблюдатель: %v", err)
	}

	// Инициализируем тестовое событие с полями, которые ДОЛЖНЫ и НЕ ДОЛЖНЫ улететь в сеть
	event := audit.Event{
		TS:         time.Now().Unix(),
		UserID:     "user_test_123",
		Action:     "shorten",
		URL:        "https://example.com",
		Method:     "POST",         // json:"-"
		Path:       "/api/shorten", // json:"-"
		StatusCode: 201,            // json:"-"
	}

	// Флаг управления состоянием эмулируемого сервера аудита (работает / упал)
	var serverFail bool

	// Создаем мок-транспорт, передавая ему expectedEvent для побайтовой сверки JSON
	transport := &mockTransport{
		t:             t,
		expectedEvent: event,
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			if serverFail {
				return nil, errors.New("network connection timeout")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		},
	}
	observer.client.SetTransport(transport)
	ctx := context.Background()

	// Запускаем весь сценарий внутри пузыря synctest.Test.
	// Любые вызовы time.Sleep или time.After внутри OnRequest будут
	// перематывать виртуальные часы Go мгновенно, не нагружая процессор.
	synctest.Test(t, func(t *testing.T) {

		// ==========================================
		// 1. ШТАТНЫЙ РЕЖИМ (CLOSED)
		// ==========================================
		observer.OnRequest(ctx, event)
		if observer.cb.state != StateClosed {
			t.Errorf("Ожидалось исходное состояние CLOSED, получено: %s", observer.cb.state)
		}
		// Проверяем: был совершен ровно 1 успешный вызов с валидацией JSON внутри мока
		if transport.calls.Load() != 1 {
			t.Errorf("Ожидался 1 сетевой вызов, произошло: %d", transport.calls.Load())
		}

		// ==========================================
		// 2. СЕРВЕР ПАДАЕТ -> ПЕРЕХОД В StateOpen
		// ==========================================
		serverFail = true
		transport.calls.Store(0) // Сбрасываем счетчик вызовов для нового этапа

		// Отправляем запросы, пока лимит ошибок (failureThreshold = 5) не разомкнет цепь.
		for range failureThreshold {
			observer.OnRequest(ctx, event)
		}

		// Проверяем, что предохранитель сработал и цепь разомкнута
		if observer.cb.state != StateOpen {
			t.Errorf("Предохранитель должен был перейти в OPEN, текущее состояние: %s", observer.cb.state)
		}
		// В состоянии CLOSED включены ретраи (maxRetries = 3).
		// На 5 запросов при упавшем сервере транспорт должен зафиксировать ровно 15 попыток (5 * 3)!
		if transport.calls.Load() != 15 {
			t.Errorf("Ожидалось 15 попыток ретраев до размыкания цепи, зафиксировано: %d", transport.calls.Load())
		}

		// Тестируем мгновенную блокировку в OPEN (Fast Fail):
		transport.calls.Store(0)
		observer.OnRequest(ctx, event)
		// Запрос должен срезаться ДО сети, счетчик обязан остаться нулевым
		if transport.calls.Load() != 0 {
			t.Error("Ошибка: предохранитель пропустил запрос в сеть в состоянии OPEN!")
		}

		// ==========================================
		// 3. ЭМУЛЯЦИЯ ОСТЫВАНИЯ ЦЕПИ В ВИРТУАЛЬНОМ ВРЕМЕНИ
		// ==========================================
		// Честно ждем cooldownTimeout (30 секунд) + 1 секунду запаса.
		// Благодаря synctest виртуальное время прокручивается вперед ЗА ДОЛИ МИЛЛИСЕКУНД.
		time.Sleep(cooldownTimeout + time.Second)

		// Помечаем, что удаленный сервер логов полностью восстановился
		serverFail = false
		transport.calls.Store(0)

		// ==========================================
		// 4. ПЕРЕХОД В StateHalfOpen И ВЫЗДОРОВЛЕНИЕ (CLOSED)
		// ==========================================
		// Следующий запрос переводит систему в HALF-OPEN для отправки тестового лога
		observer.OnRequest(ctx, event)

		if observer.cb.state != StateHalfOpen {
			t.Errorf("Предохранитель должен был перейти в HALF-OPEN, текущее состояние: %s", observer.cb.state)
		}

		// Нам нужно зафиксировать successThreshold (3) успешных ответов подряд для полного закрытия цепи.
		// Первый успешный шаг уже сделан строкой выше. Делаем оставшиеся 2 (используя for range).
		for range successThreshold - 1 {
			observer.OnRequest(ctx, event)
		}

		// Цепь обязана вернуться в исходный рабочий стейт CLOSED
		if observer.cb.state != StateClosed {
			t.Errorf("Ожидалось успешное восстановление цепи в CLOSED, текущее состояние: %s", observer.cb.state)
		}
		// Суммарно на этапе HALF-OPEN должно произойти ровно 3 успешных вызова.
		// И главное — никаких ретраев, строго по одному выстрелу на каждый OnRequest!
		if transport.calls.Load() != 3 {
			t.Errorf("Ожидалось ровно 3 тестовых вызова в сеть, зафиксировано: %d", transport.calls.Load())
		}
	})
}
