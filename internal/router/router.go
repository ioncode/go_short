package router

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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
	"github.com/ioncode/go_short/internal/router/audit"
	"github.com/ioncode/go_short/internal/service"
	"github.com/ioncode/go_short/pkg"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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

func Serve(ctx context.Context, config *config.Config) error {
	router, repo, auditor := SetupRouter(ctx, config)
	defer repo.Close()
	srv := &http.Server{
		Addr:    config.ServerAddress,
		Handler: logger.ResponseLogger(logger.RequestLogger(router)),
	}

	// Создаем локальную группу ошибок для отслеживания параллельных процессов веб-слоя
	eg, localCtx := errgroup.WithContext(ctx)

	// 1. Запуск HTTP-сервера
	eg.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("Ошибка запуска HTTP сервера: %w", err)
		}
		return nil
	})

	// 2. Ожидание сигнала отмены контекста и последующий Graceful Shutdown
	eg.Go(func() error {
		<-localCtx.Done()

		log.Println("HTTP сервер и аудитор получили сигнал остановки")

		var shutdownErr error

		// Останавливаем HTTP-сервер
		httpCtx, cancelHttp := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelHttp()
		if err := srv.Shutdown(httpCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("Ошибка остановки HTTP сервера: %w", err))
		}

		// Останавливаем аудитор
		if err := auditor.Shutdown(5 * time.Second); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("Ошибка остановки аудитора: %w", err))
		}

		return shutdownErr
	})

	// Ждем завершения процессов веб-компонента.
	// Если ListenAndServe упадет, eg.Wait() сразу вернет ошибку в main.
	return eg.Wait()
}

func SetupRouter(ctx context.Context, config *config.Config) (http.Handler, service.SiteRepository, *audit.Auditor) {
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

	// Передаем родительский ctx в аудитор
	auditor := audit.New(ctx, logger.Log, 500, 4)

	// Динамически подключаем аудит в файл, если передан параметр
	if config.AuditFile != "" {
		fileObs, err := audit.NewFileObserver(config.AuditFile, logger.Log)
		if err != nil {
			logger.Log.Error("Не удалось подключить файловый приемник аудита", zap.Error(err))
		} else {
			auditor.Register(fileObs)
			logger.Log.Info("Файловый приемник аудита успешно подключен")
		}
	}

	// и в сетевой приемник
	if config.AuditURL != "" {
		remoteObs := audit.NewRemoteObserver(config.AuditURL, logger.Log)

		// Регистрируем сетевого наблюдателя в центральном диспетчере
		auditor.Register(remoteObs)
		logger.Log.Info("Сетевой приемник аудита успешно подключен", zap.String("target_url", config.AuditURL))
	}

	router := chi.NewRouter().With(pkg.GzipMiddleware, requestContentLengthMiddleware, responseHeadersMiddleware, authMiddleware.EnsureUserHasID)
	router.With(auditor.Middleware).Get("/{alias}", handler.Get(service))
	router.Get("/ping", handler.Ping(repo))
	router.With(chiMiddleware.AllowContentType("text/plain"), auditor.Middleware).Post("/", handler.Post(service, config.ShortBaseUrl))
	router.With(chiMiddleware.AllowContentType("application/json"), auditor.Middleware).Post("/api/shorten", handler.APIPost(service, config.ShortBaseUrl))
	router.With(chiMiddleware.AllowContentType("application/json")).Post("/api/shorten/batch", handler.APIPostBatch(service, config.ShortBaseUrl))
	router.Get("/api/user/urls", handler.GetUserSites(service, config.ShortBaseUrl))
	router.Delete("/api/user/urls", handler.AsyncDeleteUserSites(service))
	return router, repo, auditor
}
