package audit

import (
	"context"
	"net/http"
	"time"

	"github.com/ioncode/go_short/pkg"
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

// Middleware возвращает chi-совместимый обработчик промежуточного ПО.
func (a *Auditor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Берем готовый контейнер из пула вместо аллокации нового
		container := getAuditContainer()

		// Гарантируем возврат контейнера в пул при любом исходе (даже при панике хендлера)
		defer container.release()

		wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
		ctx := context.WithValue(r.Context(), auditDataKey, container)

		// Передаем управление дальше по цепочке
		next.ServeHTTP(wrapper, r.WithContext(ctx))

		// Фильтруем по статус-кодам
		if wrapper.statusCode >= 200 && wrapper.statusCode < 400 {
			var userID string
			if user, err := pkg.UserFromContext(r.Context()); err == nil && user != nil {
				userID = user.ID
			}

			// 2. Делаем быстрый снимок данных
			customDataCopy := container.snapshot()

			var action Action
			if act, ok := customDataCopy[actionInternalKey].(Action); ok {
				action = act
				delete(customDataCopy, actionInternalKey)
			}

			var longURL string
			if u, ok := customDataCopy[urlInternalKey].(string); ok {
				longURL = u
				delete(customDataCopy, urlInternalKey)
			}

			// Отправляем изолированную копию в асинхронный воркер
			a.Notify(Event{
				TS:         time.Now().Unix(),
				UserID:     userID,
				Action:     action,
				URL:        longURL,
				Method:     r.Method,
				Path:       r.URL.Path,
				StatusCode: wrapper.statusCode,
				CustomData: customDataCopy,
			})
		}
	})
}
