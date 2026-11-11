package audit

import (
	"net/http"
	"sync"
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

var wrapperPool = sync.Pool{
	New: func() any {
		return &responseWriterWrapper{}
	},
}

func getResponseWrapper() *responseWriterWrapper {
	return wrapperPool.Get().(*responseWriterWrapper)
}

func (rw *responseWriterWrapper) release() {
	rw.ResponseWriter = nil
	rw.statusCode = 0
	wrapperPool.Put(rw)
}

// Middleware возвращает chi-совместимый обработчик промежуточного ПО.
func (a *Auditor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapper := getResponseWrapper()
		defer wrapper.release()
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
