package pkg

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/ioncode/go_short/internal/model"
)

// Ошибки пакета
var ErrUserNotFound = errors.New("user not found in context")

// 1. Приватный тип для ключа контекста — исключает конфликты с другими пакетами
type contextKey string

// 2. Приватная константа с уникальным типом
const userKey contextKey = "user"

// 3. Функция-сеттер для безопасного добавления пользователя в контекст, экспортируется для использования в автотестах
func WithUser(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// 4. Функция-геттер с проверкой наличия и корректности типа
func UserFromContext(ctx context.Context) (*model.User, error) {
	user, ok := ctx.Value(userKey).(*model.User)
	if !ok || user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

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

		if err == nil {
			//Если кука присутствует в запросе, но не содержит ID пользователя
			if cookie.Value == "" {
				http.Error(w, "Кука не содержит ID пользователя", http.StatusUnauthorized)
				return
			}
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

		// 4. Передаем пользователя в контекст запроса
		claims := &model.User{
			ID: userID,
		}

		ctx := WithUser(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
