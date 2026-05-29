package handler

import (
	"fmt"
	"log"
	"net/http"

	"github.com/ioncode/go_short/internal/model"
)

type GetService interface {
	Get(alias model.ShortUrl) (model.Site, error)
}

func Get(s GetService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		path := req.PathValue("alias")
		log.Println("Started get request handler with path " + path)
		alias := model.ShortUrl(path)
		log.Println("alias: " + alias)
		site, error := s.Get(alias)
		fmt.Printf("%#v\n", site)

		if error != nil {
			http.Error(res, string(alias)+": "+error.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(res, req, string(site.Url), http.StatusTemporaryRedirect)
	}
}
