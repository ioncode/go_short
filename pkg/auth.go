package pkg

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/ioncode/go_short/internal/model"
)

const cookieName = "user_id"

// AuthMiddleware хранит в себе инстанс securecookie с секретным ключом
type AuthMiddleware struct {
	sc *securecookie.SecureCookie
}

// NewAuthMiddleware создает новый экземпляр middleware
func NewAuthMiddleware(sc *securecookie.SecureCookie) *AuthMiddleware {
	return &AuthMiddleware{sc: sc}
}

// EnsureUserHasID проверяет подписанную куку. Если её нет или подпись невалидна — генерирует новый ID.
func (am *AuthMiddleware) EnsureUserHasID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID string
		var needsNewCookie bool

		// 1. Пытаемся прочитать существующую куку
		cookie, err := r.Cookie(cookieName)

		if err == nil && cookie.Value != "" {
			// 2. Декодируем и ПРОВЕРЯЕМ симметричную подпись
			// Если злоумышленник изменил userID в браузере, Decode вернет ошибку
			err = am.sc.Decode(cookieName, cookie.Value, &userID)
			if err != nil {
				// Подпись невалидна (куку подделали). Сгенерируем новый ID поверх старого.
				needsNewCookie = true
			}
		} else {
			// Куки вообще не было
			needsNewCookie = true
		}

		// 3. Если куки не было или она была "битой" — создаем заново
		if needsNewCookie {
			userID = uuid.New().String()

			// Кодируем userID и подписываем его секретным ключом
			encoded, err := am.sc.Encode(cookieName, userID)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Создаем HTTP-куку с подписанным значением
			newCookie := &http.Cookie{
				Name:     cookieName,
				Value:    encoded, // Здесь будет строка вида "строка.подпись"
				Path:     "/",
				Expires:  time.Now().Add(365 * 24 * time.Hour),
				HttpOnly: true,
				Secure:   false, // В продакшене обязательно true (только HTTPS)
				SameSite: http.SameSiteLaxMode,
			}

			http.SetCookie(w, newCookie)
		}

		// 4. Передаем чистый, проверенный userID в контекст запроса
		claims := &model.User{
			ID: userID,
		}

		ctx := context.WithValue(r.Context(), model.UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
