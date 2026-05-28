package handler

import (
	"log"
	"net/http"

	"github.com/ioncode/go_short/internal/model"
)

type GetService interface {
	Get(alias model.ShortUrl) (model.Site, error)
}

func Get(s GetService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		alias := model.ShortUrl(req.PathValue("alias"))
		log.Println("Started get request handler with alias " + alias)
		site, error := s.Get(alias)

		if error != nil {
			http.Error(res, string(alias)+": "+error.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(res, req, string(site.Url), http.StatusTemporaryRedirect)
	}
}
