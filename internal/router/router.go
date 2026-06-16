package router

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/handler"
	"github.com/ioncode/go_short/internal/logger"
	"github.com/ioncode/go_short/internal/repository"
	"github.com/ioncode/go_short/internal/service"
)

func responseHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "text/plain")
		next.ServeHTTP(w, r)
	})
}

func requestContentLengthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Middleware processing request with length ", r.ContentLength)
		if r.ContentLength > 700 {
			http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Serve(config config.Config) {
	router := SetupRouter(config)
	log.Fatal(http.ListenAndServe(config.ServerAddress, logger.ResponseLogger(logger.RequestLogger(requestContentLengthMiddleware(responseHeadersMiddleware(router))))))
}

func SetupRouter(config config.Config) http.Handler {
	repo := repository.NewMapRepository()
	service := service.NewShortner(repo)

	router := chi.NewRouter()
	router.Get("/{alias}", handler.Get(service))
	router.With(chiMiddleware.AllowContentType("text/plain")).Post("/", handler.Post(service, config.ShortBaseUrl))
	return router
}
