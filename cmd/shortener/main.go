package main

import (
	"log"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/logger"
	"github.com/ioncode/go_short/internal/router"
	"go.uber.org/zap"
)

func main() {
	config := config.ParseFlags()
	log.Println("Running server on", config.ServerAddress)
	if err := logger.Initialize("INFO"); err != nil {
		log.Panic("Logger init failed", err)
	}
	logger.Log.Info("Running server", zap.String("address", config.ServerAddress), zap.String("storage_path", config.StoragePath), zap.String("DataBase_DSN", config.DataBaseDSN))
	router.Serve(config)

}
