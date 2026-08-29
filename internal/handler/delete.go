package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/service"
	"github.com/ioncode/go_short/pkg"
)

type DeleteService interface {
	Enqueue(task service.DeleteTask)
}

func AsyncDeleteUserSites(s DeleteService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}
		aliases := []model.ShortUrl{}
		err = json.NewDecoder(req.Body).Decode(&aliases)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusBadRequest)
			return
		}

		s.Enqueue(service.DeleteTask{
			Author:  *user,
			Aliases: aliases,
		})

		res.WriteHeader(http.StatusAccepted)
	}
}
