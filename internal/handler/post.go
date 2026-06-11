package handler

import (
	"io"
	"log"
	"net/http"

	"github.com/ioncode/go_short/internal/model"
)

type ShortService interface {
	Short(url model.Url) (model.ShortUrl, error)
}

func Post(s ShortService, shortBaseURL string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		log.Println("Started Post handler")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		alias, err := s.Short(model.Url(body))
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		res.WriteHeader(http.StatusCreated)
		res.Write([]byte(model.ShortUrl(shortBaseURL) + alias))
	}
}
