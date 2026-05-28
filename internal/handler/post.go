package handler

import (
	"io"
	"net/http"

	"github.com/ioncode/go_short/internal/model"
)

type ShortService interface {
	Short(url model.Url) (model.ShortUrl, error)
}

func Post(s ShortService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		body, error := io.ReadAll(req.Body)
		if error != nil {
			http.Error(res, error.Error(), http.StatusBadRequest)
		}
		alias, error := s.Short(model.Url(body))
		if error != nil {
			http.Error(res, error.Error(), http.StatusBadRequest)
		}
		res.WriteHeader(http.StatusCreated)
		res.Write([]byte("http://localhost:8080/" + alias))
	}
}
