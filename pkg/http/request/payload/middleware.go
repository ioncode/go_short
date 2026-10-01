package payload

import (
	"context"
	"net/http"
)

// Middleware выполняет роль «входных ворот» транспортного конвейера.
// Она извлекает структуру Payload из глобального пула, один раз оборачивает
// системный контекст и гарантирует возврат памяти в пул после завершения запроса.
//
// Эта мидлварь должна быть установлена СТРОГО первой в списке глобальных middleware
// вашего роутера (например, в r.Use() фреймворка chi).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Извлекаем контейнер из пула (0 аллокаций памяти в куче)
		payloadContainer := Get()

		// 2. defer гарантирует выполнение Release() и возврат структуры в пул
		// строго после того, как запрос пройдет все хендлеры и полностью запишется в сеть
		defer payloadContainer.Release()

		// 3. Создаем единственный valueCtx на весь жизненный цикл этого запроса
		ctx := context.WithValue(r.Context(), PayloadContextKey, payloadContainer)

		// 4. Передаем управление дальше по конвейеру с обновленным контекстом
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
