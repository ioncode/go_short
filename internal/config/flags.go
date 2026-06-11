package config

import "flag"

type Config struct {
	ServerAddress string
	ShortBaseUrl  string
}

func ParseFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.ServerAddress, "a", ":8080", "адрес запуска HTTP-сервера")
	flag.StringVar(&cfg.ShortBaseUrl, "b", "http://localhost:8080/", " базовый адрес результирующего сокращённого URL")
	flag.Parse()
	return cfg
}
