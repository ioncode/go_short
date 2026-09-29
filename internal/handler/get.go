package handler

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/router/audit"
	"github.com/ioncode/go_short/pkg"
)

type GetService interface {
	Get(alias model.ShortUrl) (model.Site, error)
}

func Get(s GetService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		path := chi.URLParam(req, "alias")
		alias := model.ShortUrl(path)
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
		audit.SetURL(req, string(site.Url))

		http.Redirect(res, req, string(site.Url), http.StatusTemporaryRedirect)
	}
}

type GetByUser interface {
	GetByUser(userId string) ([]model.UserSitesResponseItem, error)
}

func GetUserSites(s GetByUser, shortBaseURL *url.URL) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}

		records, err := s.GetByUser(user.ID)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(records) == 0 {
			res.WriteHeader(http.StatusNoContent)
			return
		}

		u := *shortBaseURL
		for i, record := range records {
			u.Path = string(record.Alias)
			records[i].Alias = model.ShortUrl(u.String())
		}

		json.NewEncoder(res).Encode(records)
	}
}
