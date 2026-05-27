package handler

import (
	"net/http"

	"github.com/ioncode/go_short/internal/model"
)

type GetService interface {
	Get(alias model.ShortUrl) (model.Site, error)
}

func Get(s GetService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		site, error := s.Get("dfdf")

		if error != nil {
			http.Error(res, error.Error(), 500)
			return
		}
		res.Write([]byte(site.Url))
	}
}
