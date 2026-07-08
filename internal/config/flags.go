package config

import (
	"flag"
	"os"
)

type Config struct {
	ServerAddress string
	ShortBaseUrl  string
	StoragePath   string
}

func ParseFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.ServerAddress, "a", ":8080", "адрес запуска HTTP-сервера")
	flag.StringVar(&cfg.ShortBaseUrl, "b", "http://localhost:8080/", " базовый адрес результирующего сокращённого URL")
	flag.StringVar(&cfg.StoragePath, "f", "storage.json", "путь к файлу для сохранения сайтов")
	flag.Parse()

	if envServerAddress := os.Getenv("SERVER_ADDRESS"); envServerAddress != "" {
		cfg.ServerAddress = envServerAddress
	}

	if envBaseUrl := os.Getenv("BASE_URL"); envBaseUrl != "" {
		cfg.ShortBaseUrl = envBaseUrl
	}

	if envStoragePath := os.Getenv("FILE_STORAGE_PATH"); envStoragePath != "" {
		cfg.StoragePath = envStoragePath
	}

	return cfg
}
