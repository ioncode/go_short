package handler

import "net/http"

func Get(res http.ResponseWriter, req *http.Request) {
	res.Write([]byte("Это GET"))
}
