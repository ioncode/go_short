package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ioncode/go_short/internal/model"
)

type GetService interface {
	Get(alias model.ShortUrl) (model.Site, error)
}

func Get(s GetService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		path := chi.URLParam(req, "alias")
		log.Println("Started get request handler with path " + path)
		alias := model.ShortUrl(path)
		site, err := s.Get(alias)

		if err != nil {
			http.Error(res, string(alias)+": "+err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(res, req, string(site.Url), http.StatusTemporaryRedirect)
	}
}

type GetByUser interface {
	GetByUser(userId string) ([]model.UserSitesResponseItem, error)
}

func GetUserSites(s GetByUser) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		user, ok := req.Context().Value(model.UserContextKey).(model.User)
		if !ok {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}
		log.Println("Started user get request handler " + user.ID)

		records, err := s.GetByUser(user.ID)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusInternalServerError)
		}

		if len(records) == 0 {
			res.WriteHeader(http.StatusNoContent)
			return
		}

		json.NewEncoder(res).Encode(records)
	}
}
