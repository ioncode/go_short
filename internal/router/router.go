package router

import (
	"log"
	"net/http"

	"github.com/ioncode/go_short/internal/handler"
)

func Serve() {
	mux := http.NewServeMux()

	mux.HandleFunc(`GET /`, handler.Get)
	mux.HandleFunc(`POST /`, handler.Post)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
