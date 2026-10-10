// Package payload предоставляет легковесный, потокобезопасный и переиспользуемый
// контейнер для хранения полезной нагрузки (метаданных) в рамках одного HTTP-запроса.
//
// Использование пакета позволяет избежать неявных аллокаций памяти в куче (Zero-Alloc)
// при передаче данных (таких как ID авторизованного пользователя или логи аудита)
// сквозь цепочки middleware и хендлеров транспортного слоя, заменяя создание новых
// системных контекстов модификацией полей существующего объекта по указателю.
// ⚠️ ВАЖНОЕ ОГРАНИЧЕНИЕ ПО БЕЗОПАСНОСТИ (RACE CONDITION):
// Контейнер Payload живет строго в рамках жизненного цикла текущего HTTP-запроса.
// ЗАПРЕЩЕНО использовать контекст запроса с этим payload внутри фоновых горутин (go func).
// После отправки ответа клиенту контейнер очищается и возвращается в пул.
// Если вам нужны данные в горутине, скопируйте их в отдельные переменные ДО запуска горутины:
//
//	authorID := payload.GetAuthorID(r.Context())
//	go func(id uuid.UUID) { /* использование id безопасно */ }(authorID)
package payload

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// contextKey определяет приватный тип для ключа контекста,
// полностью исключающий конфликты с другими пакетами в стандартном context.Context.
type contextKey string

// PayloadContextKey — уникальный ключ, по которому мидлвари и хендлеры
// находят контейнер полезной нагрузки в системном контексте запроса.
const PayloadContextKey contextKey = "http_request_payload"

const (
	defaultMapCapacity = 4  // авторизация, аудит
	maxMapCapacity     = 64 // Защита от утечки памяти (Memory Bloat)
)

// Payload представляет собой изолированный контейнер полезной нагрузки запроса.
// Все внутренние поля строго приватны. Изменение и чтение состояния данных
// возможны только через экспортируемый потокобезопасный интерфейс пакета.
type Payload struct {
	mu       sync.RWMutex
	authorID uuid.UUID
	data     map[string]any
}

// pool реализует глобальный sync.Pool для переиспользования памяти структур Payload.
var pool = sync.Pool{
	New: func() any {
		return &Payload{
			data: make(map[string]any, defaultMapCapacity),
		}
	},
}

// Get извлекает чистый, готовый к работе экземпляр Payload из пула.
func Get() *Payload {
	return pool.Get().(*Payload)
}

// Release обнуляет все внутренние поля контейнера и возвращает его в глобальный пул.
// Очистка внутренней мапы через range-delete сохраняет её выделенную емкость (capacity)
// в куче, полностью предотвращая аллокации памяти при последующих запросах к серверу.
func (p *Payload) Release() {
	p.mu.Lock()
	p.authorID = uuid.Nil
	// Защита от раздувания памяти: если мапа стала слишком большой,
	// сбрасываем её емкость, позволяя GC очистить память.
	if len(p.data) > maxMapCapacity {
		p.data = make(map[string]any, defaultMapCapacity)
	} else {
		for k := range p.data {
			delete(p.data, k)
		}
	}
	p.mu.Unlock()
	pool.Put(p)
}

// =========================================================================
// ПОТОКОБЕЗОПАСНЫЙ И ЭКСПОРТИРУЕМЫЙ ИНТЕРФЕЙС ПАКЕТА (API)
// =========================================================================

// SetAuthorID атомарно записывает бинарный UUID авторизованного пользователя
// в контейнер полезной нагрузки текущего HTTP-запроса.
func SetAuthorID(r *http.Request, id uuid.UUID) {
	if p, ok := r.Context().Value(PayloadContextKey).(*Payload); ok {
		p.mu.Lock()
		p.authorID = id
		p.mu.Unlock()
	}
}

// GetAuthorID атомарно извлекает бинарный UUID пользователя из контекста.
// Метод рекомендуется вызывать на стыке транспортного и сервисного слоев (Service/Repository).
// Если контейнер отсутствует в контексте, возвращает uuid.Nil.
func GetAuthorID(ctx context.Context) uuid.UUID {
	if p, ok := ctx.Value(PayloadContextKey).(*Payload); ok {
		p.mu.RLock()
		id := p.authorID
		p.mu.RUnlock()
		return id
	}
	return uuid.Nil
}

// SetCustomValue атомарно сохраняет кастомные метаданные по строковому ключу.
// Используется инфраструктурными слоями (например, пакетом аудита или логирования).
func SetCustomValue(r *http.Request, key string, value any) {
	if p, ok := r.Context().Value(PayloadContextKey).(*Payload); ok {
		p.mu.Lock()
		p.data[key] = value
		p.mu.Unlock()
	}
}

// GetCustomValue атомарно считывает кастомные метаданные по ключу.
// Возвращает значение и флаг успешности поиска (bool), предотвращая паники.
func GetCustomValue(ctx context.Context, key string) (any, bool) {
	if p, ok := ctx.Value(PayloadContextKey).(*Payload); ok {
		p.mu.RLock()
		val, exists := p.data[key]
		p.mu.RUnlock()
		return val, exists
	}
	return nil, false
}
