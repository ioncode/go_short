package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/ioncode/go_short/pkg/http/request/payload"
)

var (
	ErrUserNotFound   = errors.New("user ID not found in payload container")
	ErrEmptyCookieMsg = errors.New("cookie is present but value is empty")
)

const cookieName = "user_id"

func UserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	id := payload.GetAuthorID(ctx)
	if id == uuid.Nil {
		return uuid.Nil, ErrUserNotFound
	}
	return id, nil
}

type AuthMiddleware struct {
	sc *securecookie.SecureCookie
}

func NewAuthMiddleware(sc *securecookie.SecureCookie) *AuthMiddleware {
	return &AuthMiddleware{sc: sc}
}

func (am *AuthMiddleware) EnsureUserHasID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := am.extractUserID(r)

		if err != nil {
			if errors.Is(err, ErrEmptyCookieMsg) {
				http.Error(w, "Кука не содержит ID пользователя", http.StatusUnauthorized)
				return
			}

			var genErr error
			userID, genErr = am.issueNewUserSession(w)
			if genErr != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		}

		payload.SetAuthorID(r, userID)
		next.ServeHTTP(w, r)
	})
}

func (am *AuthMiddleware) extractUserID(r *http.Request) (uuid.UUID, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return uuid.Nil, err
	}

	if cookie.Value == "" {
		return uuid.Nil, ErrEmptyCookieMsg
	}

	var userID uuid.UUID
	if err := am.sc.Decode(cookieName, cookie.Value, &userID); err != nil {
		return uuid.Nil, err
	}

	return userID, nil
}

func (am *AuthMiddleware) issueNewUserSession(w http.ResponseWriter) (uuid.UUID, error) {
	userID := uuid.New()

	encoded, err := am.sc.Encode(cookieName, userID)
	if err != nil {
		return uuid.Nil, err
	}

	newCookie := &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		Expires:  time.Now().Add(365 * 24 * time.Hour),
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, newCookie)
	return userID, nil
}
