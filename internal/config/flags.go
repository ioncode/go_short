package config

import "flag"

var ServerAddress string
var ShortBaseUrl string

func ParseFlags() {
	flag.StringVar(&ServerAddress, "a", ":8080", "адрес запуска HTTP-сервера")
	flag.StringVar(&ShortBaseUrl, "b", "http://localhost:8080/", " базовый адрес результирующего сокращённого URL")
	flag.Parse()
}
