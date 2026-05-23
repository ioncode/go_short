package router

import (
	"log"
	"net/http"

	"github.com/ioncode/go_short/internal/handler"
)

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		contentType := r.Header.Get("Content-type")
		if contentType != "text/plain" {
			http.Error(w, "Content type not correct", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Serve() {
	mux := http.NewServeMux()

	mux.HandleFunc(`GET /`, handler.Get)
	mux.HandleFunc(`POST /`, handler.Post)

	log.Fatal(http.ListenAndServe(":8080", middleware(mux)))
}
