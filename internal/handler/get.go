package handler

import (
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
