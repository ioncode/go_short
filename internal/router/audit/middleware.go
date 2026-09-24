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
		customData := make(map[string]interface{})

		ctx := context.WithValue(r.Context(), auditDataKey, customData)
		r = r.WithContext(ctx)

		wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		// Допускаются успешные ответы и любые перенаправления (200-399)
		if wrapper.statusCode >= 200 && wrapper.statusCode < 400 {
			var userID string
			if user, err := pkg.UserFromContext(r.Context()); err == nil && user != nil {
				userID = user.ID
			}

			// Извлекаем действие
			var action string
			if act, ok := customData[actionInternalKey].(string); ok {
				action = act
				delete(customData, actionInternalKey)
			}

			// Извлекаем оригинальный URL
			var longURL string
			if u, ok := customData[urlInternalKey].(string); ok {
				longURL = u
				delete(customData, urlInternalKey)
			}

			a.Notify(Event{
				Timestamp:  time.Now(),
				Method:     r.Method,
				Path:       r.URL.Path,
				StatusCode: wrapper.statusCode,
				UserID:     userID,
				Action:     action,
				URL:        longURL, // Заполняем поле URL
				CustomData: customData,
			})
		}
	})
}
