package audit

import (
	"net/http"

	"github.com/ioncode/go_short/pkg/pool"
)

// responseWriterWrapper является декоратором для стандартного http.ResponseWriter,
// позволяющим перехватить и сохранить HTTP статус-код ответа.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader переопределяет стандартный метод для фиксации статус-кода.
func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

var wrapperPool = pool.New(func() *responseWriterWrapper {
	return &responseWriterWrapper{}
})

// Reset сбрасывает поля обёртки перед возвратом в пул.
func (rw *responseWriterWrapper) Reset() {
	rw.ResponseWriter = nil
	rw.statusCode = 0
}

// Middleware возвращает chi-совместимый обработчик промежуточного ПО.
func (a *Auditor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapper := wrapperPool.Get()
		defer wrapperPool.Put(wrapper)
		wrapper.ResponseWriter = w
		wrapper.statusCode = http.StatusOK

		// Передаем управление дальше по цепочке для наполнения payload хэндлерами
		next.ServeHTTP(wrapper, r)

		// Фильтруем по статус-кодам
		if wrapper.statusCode >= 200 && wrapper.statusCode < 400 {
			// создаем событие аудита с глубокой копией для предотвращения гонок асинхронной обработки
			event, hasAction := NewEvent(r, wrapper.statusCode)
			// аудируем только те успешные запросы, где установлен экшн (ранее в хэндлере)
			if hasAction {
				a.Notify(event)
			}
		}
	})
}
