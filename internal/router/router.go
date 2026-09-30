package router

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	"github.com/ioncode/go_short/internal/router/audit/remote"
	"github.com/ioncode/go_short/internal/service"
	"github.com/ioncode/go_short/pkg"
	"github.com/ioncode/httpcodec"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
)

// Оставляем общую мидлварь только для CORS (ставим на весь роутер)
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

func Serve(ctx context.Context, config *config.Config) error {
	router, repo, auditor := SetupRouter(ctx, config)
	defer repo.Close()
	// По умолчанию отдаем чистый роутер без накладных расходов логгера
	var finalHandler http.Handler = router

	// Проверяем уровень логирования один раз при старте сервера.
	// Подключаем middleware, только если в конфиге включен уровень Info (или ниже)
	if logger.Log.Core().Enabled(zapcore.InfoLevel) {
		finalHandler = logger.ResponseLogger(logger.RequestLogger(router))
	}
	srv := &http.Server{
		Addr:    config.ServerAddress,
		Handler: finalHandler,
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
		// === НАЧАЛО ОПТИМИЗАЦИИ ПУЛА СОЕДИНЕНИЙ ===
		// 1. Ограничиваем максимальное количество ОДНОВРЕМЕННО ОТКРЫТЫХ соединений к СУБД.
		// Значение должно быть строго меньше, чем max_connections в настройках самого Postgres (например, 20-30).
		sqlDB.SetMaxOpenConns(30)

		// 2. Ограничиваем максимальное количество простаивающих (idle) соединений в пуле.
		// Это удерживает сессии открытыми, предотвращая постоянные вызовы connect/dial на каждый запрос.
		sqlDB.SetMaxIdleConns(30)

		// 3. Время жизни соединения в пуле (предотвращает утечки памяти и ресурсов на стороне СУБД)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
		// === КОНЕЦ ОПТИМИЗАЦИИ ===
		repo = repository.NewPostgresSitesRepository(sqlDB)
	}

	// not persisted random key for userAuthMiddleware
	hashKey := securecookie.GenerateRandomKey(64)

	sc := securecookie.New(hashKey, nil)

	// КРИТИЧЕСКАЯ ОПТИМИЗАЦИЯ: полностью отключаем Gob.
	// Теперь securecookie работает со строкой UUID напрямую, минуя оверхед в 111 МБ мусора!
	sc.SetSerializer(pkg.StringCodec{})

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
		remoteObs, err := remote.NewRemoteObserver(config.AuditURL, logger.Log)
		if err != nil {
			logger.Log.Error("Не удалось подключить сетевой приемник аудита", zap.Error(err))
		} else {
			auditor.Register(remoteObs)
			logger.Log.Info("Сетевой приемник аудита успешно подключен", zap.String("target_url", config.AuditURL))
		}
	}

	// 1. Инициализируем кодек v0.0.4 через Functional Options.
	// Выставляем жесткий лимит входящего тела в 8 КБ (8192 байта) для защиты от OOM.
	// Настраиваем лимиты очистки мусора Keep-Alive сокетов и капы буферов ответов.
	codec := httpcodec.New(
		8192,
		httpcodec.WithMaxTrashRead(64*1024), // Очистка до 64 КБ для сохранения Keep-Alive сессий
		httpcodec.WithInitJSONBufferCap(4*1024),  // Старт буфера ответа с 4 КБ
		httpcodec.WithMaxJSONBufferCap(256*1024), // Защита от OOM: жесткий лимит буфера ответа 256 КБ
	)

	// 2. Собираем базовую цепочку Middleware.
	// codec.Middleware() автоматически синхронизирует MaxBytesReader сокета под лимит 8 КБ
	router := chi.NewRouter().With(
		pkg.GzipMiddleware,
		corsMiddleware,
		codec.Middleware(), // Автоматическая DoS-защита периметра на уровне сокета
		authMiddleware.EnsureUserHasID,
	)
	// 3. Регистрируем эндпоинты и пробрасываем кодек как зависимость во все POST-фабрики
	router.With(auditor.Middleware).Get("/{alias}", handler.Get(service))
	router.Get("/ping", handler.Ping(repo))

	// Текстовый эндпоинт (Вход: text/plain, Выход: text/plain)
	router.With(chiMiddleware.AllowContentType("text/plain"), auditor.Middleware).Post("/", handler.Post(service, config.ShortBaseUrl, codec))

	// Одиночный REST API эндпоинт (Вход: JSON, Выход: JSON)
	router.With(chiMiddleware.AllowContentType("application/json"), auditor.Middleware).Post("/api/shorten", handler.APIPost(service, config.ShortBaseUrl, codec))

	// Пакетный REST API эндпоинт батчей (Вход: JSON-массив, Выход: JSON-массив)
	router.With(chiMiddleware.AllowContentType("application/json")).Post("/api/shorten/batch", handler.APIPostBatch(service, config.ShortBaseUrl, codec))

	// Список сайтов пользователя (Выход: JSON-массив)
	router.Get("/api/user/urls", handler.GetUserSites(service, config.ShortBaseUrl, codec))

	// запрос на пакетное удаление ссылок пользователя (Вход: JSON-массив, Выход: JSON-ошибка авторизации или заголовк 202)
	router.Delete("/api/user/urls", handler.AsyncDeleteUserSites(service, codec))

	return router, repo, auditor
}
