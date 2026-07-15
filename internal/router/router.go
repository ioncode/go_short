package router

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/handler"
	"github.com/ioncode/go_short/internal/logger"
	"github.com/ioncode/go_short/internal/repository"
	"github.com/ioncode/go_short/internal/service"
	"github.com/ioncode/go_short/pkg"
)

func responseHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if strings.HasPrefix(r.RequestURI, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}

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
	router, repo := SetupRouter(config)
	defer repo.Close()
	log.Fatal(http.ListenAndServe(config.ServerAddress, logger.ResponseLogger(logger.RequestLogger(router))))
}

func SetupRouter(config config.Config) (http.Handler, service.SiteRepository) {
	var repo service.SiteRepository
	if config.DataBaseDSN == "" {
		repo = repository.NewMapRepository(config.StoragePath)
	} else {
		db, err := sql.Open("pgx", config.DataBaseDSN)
		if err != nil {
			log.Fatalf("Failed to open connection: %v", err)
		}

		m, err := migrate.New(
			"file://../../migrations",
			config.DataBaseDSN)
		if err != nil {
			log.Fatal(err)
		}
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		repo = repository.NewPostgresSitesRepository(db)
	}

	service := service.NewShortner(repo)

	router := chi.NewRouter().With(pkg.GzipMiddleware, requestContentLengthMiddleware, responseHeadersMiddleware)
	router.Get("/{alias}", handler.Get(service))
	router.Get("/ping", handler.Ping(repo))
	router.With(chiMiddleware.AllowContentType("text/plain")).Post("/", handler.Post(service, config.ShortBaseUrl))
	router.With(chiMiddleware.AllowContentType("application/json")).Post("/api/shorten", handler.APIPost(service, config.ShortBaseUrl))
	return router, repo
}
