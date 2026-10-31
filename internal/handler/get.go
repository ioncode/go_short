package handler

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/pkg/http/audit"
	"github.com/ioncode/go_short/pkg/http/auth"

	"github.com/ioncode/httpcodec"
)

// GetService определяет интерфейс для поиска сайта по короткому алиасу.
type GetService interface {
	Get(alias model.ShortURL) (model.Site, error)
}

// Get обрабатывает HTTP-запросы GET /{alias} для редиректа на оригинальный URL.
func Get(s GetService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		path := chi.URLParam(req, "alias")
		alias := model.ShortURL(path)
		site, err := s.Get(alias)
		if err != nil {
			http.Error(res, string(alias)+": "+err.Error(), http.StatusBadRequest)
			return
		}

		if site.DeletedFlag {
			res.WriteHeader(http.StatusGone)
			return
		}

		audit.SetAction(req, audit.ActionFollow)
		audit.SetURL(req, string(site.URL))

		http.Redirect(res, req, string(site.URL), http.StatusTemporaryRedirect)
	}
}

// GetByUser определяет интерфейс для получения списка всех сайтов конкретного пользователя.
type GetByUser interface {
	GetByUser(userID uuid.UUID) ([]model.UserSitesResponseItem, error)
}

// GetUserSites возвращает JSON-список всех сокращенных ссылок авторизованного пользователя.
//
// Метод полностью оптимизирован: сериализация массива records
// и отправка структурированных JSON-ошибок переведены на метод codec.WriteJSON.
// Использование jsonPool библиотеки полностью обнуляет накладные расходы на маршалинг ответа.
func GetUserSites(s GetByUser, shortBaseURL *url.URL, codec *httpcodec.Codec) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		userID, err := auth.UserIDFromContext(req.Context())
		if err != nil {
			writeJSONError(res, "Ошибка авторизации", http.StatusUnauthorized, codec)
			return
		}

		records, err := s.GetByUser(userID)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusInternalServerError, codec)
			return
		}

		if len(records) == 0 {
			res.WriteHeader(http.StatusNoContent) // 204 NoContent
			return
		}

		u := *shortBaseURL
		// Используем цикл по индексу для оптимизации
		for i := range records {
			u.Path = string(records[i].Alias)
			records[i].Alias = model.ShortURL(u.String())
		}

		// Потоковая отправка JSON-ответа из пула буферов.
		// Заголовок Content-Type: application/json выставляется кодеком автоматически.
		codec.WriteJSON(res, http.StatusOK, &records)
	}
}
