package main

import (
	"log"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/router"
)

func main() {
	config := config.ParseFlags()
	log.Println("Running server on", config.ServerAddress)
	router.Serve(config)
}
