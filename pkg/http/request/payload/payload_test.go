package payload_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/ioncode/go_short/pkg/http/request/payload"
)

// 1. UNIT-ТЕСТЫ: Проверка базовой логики Set/Get
func TestPayload_SetAndGetAuthorID(t *testing.T) {
	// Создаем тестовый HTTP-запрос и оборачиваем его нашей мидлварью
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler := payload.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Генерируем тестовый UUID
		expectedID := uuid.New()

		// Записываем ID через наш API
		payload.SetAuthorID(r, expectedID)

		// Считываем ID из контекста
		actualID := payload.GetAuthorID(r.Context())

		if actualID != expectedID {
			t.Errorf("Ожидался AuthorID %v, получен %v", expectedID, actualID)
		}
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestPayload_SetAndGetCustomValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", nil)
	handler := payload.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем запись различных типов данных (Action и string)
		payload.SetCustomValue(r, "action", "SHORTEN")
		payload.SetCustomValue(r, "url", "https://google.com")

		// Читаем action
		val, exists := payload.GetCustomValue(r.Context(), "action")
		if !exists || val != "SHORTEN" {
			t.Errorf("Не удалось корректно извлечь 'action'")
		}

		// Читаем url
		val, exists = payload.GetCustomValue(r.Context(), "url")
		if !exists || val != "https://google.com" {
			t.Errorf("Не удалось корректно извлечь 'url'")
		}

		// Проверяем несуществующий ключ
		_, exists = payload.GetCustomValue(r.Context(), "non_existent")
		if exists {
			t.Errorf("Геттер вернул true для несуществующего ключа")
		}
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestPayload_EmptyContext(t *testing.T) {
	// Проверяем поведение хелперов, если мидлварь забыли подключить (Защита от паник)
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Запись в сырой запрос не должна приводить к panic
	payload.SetAuthorID(req, uuid.New())
	payload.SetCustomValue(req, "test", 123)

	// Чтение должно безопасно возвращать дефолтные нулевые значения
	if id := payload.GetAuthorID(ctx); id != uuid.Nil {
		t.Errorf("Ожидался uuid.Nil для пустого контекста, получен %v", id)
	}

	if _, exists := payload.GetCustomValue(ctx, "test"); exists {
		t.Errorf("Ожидалось false для пустого контекста")
	}
}

// 2. CONCURRENCY ТЕСТЫ: Проверка на состояние гонки данных (Data Races)
func TestPayload_ConcurrencyRace(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler := payload.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wg sync.WaitGroup
		workers := 100

		// Запускаем параллельную конкурентную запись и чтение из разных горутин
		wg.Add(workers * 2)
		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				payload.SetAuthorID(r, uuid.New())
				payload.SetCustomValue(r, "key", "value")
			}()
			go func() {
				defer wg.Done()
				_ = payload.GetAuthorID(r.Context())
				_, _ = payload.GetCustomValue(r.Context(), "key")
			}()
		}
		wg.Wait()
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)
}

// 3. БЕНЧМАРКИ: Проверяем эффективность аллокаций в куче
func BenchmarkPayload_Lifecycle(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	testID := uuid.New()

	// Наш хендлер имитирует реальную Highload-нагрузку
	h := payload.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload.SetAuthorID(r, testID)
		payload.SetCustomValue(r, "action", "FOLLOW")

		_ = payload.GetAuthorID(r.Context())
		_, _ = payload.GetCustomValue(r.Context(), "action")
	}))

	rec := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs() // Включаем детальный отчет об аллокациях памяти
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(rec, req)
	}
}

// Приватные ключи для нативного контекста, чтобы сравнение было честным
type nativeKey string

const (
	nativeUserKey   nativeKey = "user"
	nativeActionKey nativeKey = "action"
)

// BenchmarkNative_ContextLifecycle имитирует старый подход:
// создание новых контекстов через WithValue на каждом слое.
func BenchmarkNative_ContextLifecycle(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	testID := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// 1. Слой авторизации создает новый контекст
		ctx1 := context.WithValue(req.Context(), nativeUserKey, testID)

		// 2. Слой аудита создает ЕЩЕ ОДИН контекст поверх первого
		ctx2 := context.WithValue(ctx1, nativeActionKey, "FOLLOW")

		// 3. Имитируем чтение данных в хендлере и сервисе
		_ = ctx2.Value(nativeUserKey).(uuid.UUID)
		_ = ctx2.Value(nativeActionKey).(string)
	}
}

func BenchmarkPayload_HTTP_Lifecycle(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	testID := uuid.New()

	// Имитируем мидлварь авторизации, которая пишет в пул
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload.SetAuthorID(r, testID)
			next.ServeHTTP(w, r)
		})
	}

	// Имитируем мидлварь аудита, которая пишет в пул
	auditMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload.SetCustomValue(r, "action", "FOLLOW")
			next.ServeHTTP(w, r)
		})
	}

	// Финальный хендлер читает из пула
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = payload.GetAuthorID(r.Context())
		_, _ = payload.GetCustomValue(r.Context(), "action")
	})

	// Собираем сквозной конвейер. payload.Middleware ВСЕГДА стоит первой!
	pipeline := payload.Middleware(authMiddleware(auditMiddleware(handler)))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pipeline.ServeHTTP(rec, req)
	}
}

// 2. БЕНЧМАРК НАТИВНОГО КОНТЕКСТА (Сборка цепочки ДО таймера)
func BenchmarkNative_HTTP_Lifecycle(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	testID := uuid.New()

	// Имитируем мидлварь авторизации через WithValue
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), nativeUserKey, testID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	// Имитируем мидлварь аудита через WithValue
	auditMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), nativeActionKey, "FOLLOW")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	// Финальный хендлер читает из нативного контекста
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Context().Value(nativeUserKey).(uuid.UUID)
		_ = r.Context().Value(nativeActionKey).(string)
	})

	// Собираем нативный конвейер
	pipeline := authMiddleware(auditMiddleware(handler))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pipeline.ServeHTTP(rec, req)
	}
}

// =========================================================================
// ДИНАМИЧЕСКИЕ БЕНЧМАРКИ ДЛЯ СЛОЖНЫХ КОНВЕЙЕРОВ (10 и 100 мидлварей)
// =========================================================================

// 1. НАШ ПАКЕТ: Генерирует цепочку из N мидлварей, пишущих в одну шину
func BenchmarkPayload_HTTP_Depth10(b *testing.B)  { runPayloadDepthBenchmark(b, 10) }
func BenchmarkPayload_HTTP_Depth100(b *testing.B) { runPayloadDepthBenchmark(b, 100) }

func runPayloadDepthBenchmark(b *testing.B, depth int) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	// Базовый хендлер — читаем последний добавленный ключ,
	// чтобы проверить честный поиск по всей глубине мапы!
	lastKey := "key-" + string(rune(depth-1))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = payload.GetCustomValue(r.Context(), lastKey)
	})

	// Оборачиваем хендлер в depth-количество мидлварей
	var pipeline http.Handler = handler
	for i := range depth {
		key := "key-" + string(rune(i))

		// Явно передаем key аргументом k во внешнюю функцию.
		// Это создает изолированную копию строки в стек-кадре каждой мидлвари!
		pipeline = func(next http.Handler, k string) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload.SetCustomValue(r, k, "value")
				next.ServeHTTP(w, r)
			})
		}(pipeline, key) // Пробрасываем key сюда
	}

	// Payload.Middleware ВСЕГДА стоит самой первой и вызывается ОДИН РАЗ
	finalPipeline := payload.Middleware(pipeline)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		finalPipeline.ServeHTTP(rec, req)
	}
}

// 2. НАТИВНЫЙ КОНТЕКСТ: Генерирует цепочку из N мидлварей, плодящих WithValue
func BenchmarkNative_HTTP_Depth10(b *testing.B)  { runNativeDepthBenchmark(b, 10) }
func BenchmarkNative_HTTP_Depth100(b *testing.B) { runNativeDepthBenchmark(b, 100) }

func runNativeDepthBenchmark(b *testing.B, depth int) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	// Находим последний сгенерированный ключ, чтобы замерить худший случай
	// (worst-case) линейного поиска по дереву нативного контекста.
	lastKey := nativeKey("key-" + string(rune(depth-1)))

	// Базовый хендлер читает значение из нативного контекста
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Context().Value(lastKey)
	})

	// Оборачиваем хендлер в depth-количество мидлварей с нативным WithValue
	var pipeline http.Handler = handler
	for i := range depth {
		key := nativeKey("key-" + string(rune(i)))

		// Явно передаем key аргументом k во внешнюю функцию,
		// чтобы предотвратить захват переменной цикла в замыкании.
		pipeline = func(next http.Handler, k nativeKey) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), k, "value")
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}(pipeline, key) // Пробрасываем key как аргумент
	}

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		pipeline.ServeHTTP(rec, req)
	}
}
