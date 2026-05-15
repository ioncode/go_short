package handler

import "net/http"

func Post(res http.ResponseWriter, req *http.Request) {
	res.Write([]byte("Это POST"))
}
