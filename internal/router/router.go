package router

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/gorilla/securecookie"
	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/config/db"
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
		r.Body = http.MaxBytesReader(w, r.Body, 7000)
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
		err := db.RunPostgressMigrations(config.DataBaseDSN)
		if err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		sqlDB, err := sql.Open("pgx", config.DataBaseDSN)
		if err != nil {
			log.Fatalf("Failed to open connection: %v", err)
		}
		repo = repository.NewPostgresSitesRepository(sqlDB)
	}

	// not persisted random key for userAuthMiddleware
	hashKey := securecookie.GenerateRandomKey(64)

	sc := securecookie.New(hashKey, nil)

	authMiddleware := pkg.NewAuthMiddleware(sc)

	service := service.NewShortner(repo)

	router := chi.NewRouter().With(pkg.GzipMiddleware, requestContentLengthMiddleware, responseHeadersMiddleware, authMiddleware.EnsureUserHasID)
	router.Get("/{alias}", handler.Get(service))
	router.Get("/ping", handler.Ping(repo))
	router.With(chiMiddleware.AllowContentType("text/plain")).Post("/", handler.Post(service, config.ShortBaseUrl))
	router.With(chiMiddleware.AllowContentType("application/json")).Post("/api/shorten", handler.APIPost(service, config.ShortBaseUrl))
	router.With(chiMiddleware.AllowContentType("application/json")).Post("/api/shorten/batch", handler.APIPostBatch(service, config.ShortBaseUrl))
	router.Get("/api/user/urls", handler.GetUserSites(service, config.ShortBaseUrl))
	router.Delete("/api/user/urls", handler.AsyncDeleteUserSites(service))
	return router, repo
}
