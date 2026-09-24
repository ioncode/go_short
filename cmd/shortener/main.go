package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/logger"
	"github.com/ioncode/go_short/internal/router"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func main() {
	// 1. Загружаем флаги конфигурации
	cfg := config.ParseFlags()

	// 2. Инициализируем глобальный логгер zap
	if err := logger.Initialize("INFO"); err != nil {
		log.Panic("Не удалось инициализировать логгер: ", err)
	}
	defer logger.Log.Sync() // Сбрасываем буфер логов перед закрытием

	logger.Log.Info("Запуск приложения",
		zap.String("address", cfg.ServerAddress),
		zap.String("storage_path", cfg.StoragePath),
		zap.String("DataBase_DSN", cfg.DataBaseDSN),
	)

	// 3. Создаем каналы для перехвата сигналов ОС (Ctrl+C, kill)
	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, os.Interrupt, syscall.SIGTERM)

	rootCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	// 4. Создаем errgroup на базе фонового контекста для оркестрации компонентов
	g, gCtx := errgroup.WithContext(rootCtx)

	// 5. Запускаем Веб-сервер (внутри которого сгруппированы http.Server и Auditor)
	g.Go(func() error {
		// router.Serve блокируется, принимает контекст отмены и возвращает error
		return router.Serve(gCtx, cfg)
	})

	// 6. Блокируемся в main и ждем либо сигнала от ОС, либо отмены gCtx при ошибке в запущенных компонентах
	select {
	case sig := <-shutdownSignals:
		logger.Log.Info("Получен сигнал остановки", zap.String("signal", sig.String()))
	case <-gCtx.Done():
		logger.Log.Error("Фоновый процесс остановлен из за ошибки в запущенном компоненте", zap.Error(gCtx.Err()))
	}

	cancelApp()

	// 7. Ожидаем завершения graceful shutdown всех горутин (лимит времени 10 секунд на приложение)
	// g.Wait() каскадно отменит gCtx, запустив внутренний Shutdown для http.Server и Auditor.
	if err := g.Wait(); err != nil {
		logger.Log.Error("Приложение остановлено с ошибкой", zap.Error(err))
		os.Exit(1)
	}

	logger.Log.Info("Приложение корректно остановлено")
}
