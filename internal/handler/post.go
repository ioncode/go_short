package handler

import (
	"io"
	"net/http"
)

func Post(res http.ResponseWriter, req *http.Request) {
	res.Write([]byte("Это POST"))
	body, error := io.ReadAll(req.Body)
	if error != nil {
		http.Error(res, error.Error(), http.StatusBadRequest)
	}
	res.Write([]byte(body))
}
