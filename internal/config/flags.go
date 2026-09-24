package config

import (
	"flag"
	"os"
)

// Config содержит глобальные настройки конфигурации приложения,
// считываемые из флагов командной строки или переменных окружения.
type Config struct {
	ServerAddress string
	ShortBaseUrl  string
	StoragePath   string
	DataBaseDSN   string
	AuditFile     string
	AuditURL      string
}

// ParseFlags парсит переданные флаги командной строки и проверяет
// наличие соответствующих переменных окружения.
// Возвращает указатель на объект Config для предотвращения лишнего копирования в памяти.
func ParseFlags() *Config {
	// Инициализируем структуру через указатель
	cfg := &Config{}

	flag.StringVar(&cfg.ServerAddress, "a", ":8080", "адрес запуска HTTP-сервера")
	flag.StringVar(&cfg.ShortBaseUrl, "b", "http://localhost:8080/", " базовый адрес результирующего сокращённого URL")
	flag.StringVar(&cfg.StoragePath, "f", "storage.json", "путь к файлу для сохранения сайтов")
	flag.StringVar(&cfg.DataBaseDSN, "d", "", "параметры подключения к БД")
	flag.StringVar(&cfg.AuditFile, "audit-file", "audit.json", "путь к файлу-приёмнику логов аудита")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "URL удаленного сервера-приёмника логов аудита")
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

	if envDataBaseDSN := os.Getenv("DATABASE_DSN"); envDataBaseDSN != "" {
		cfg.DataBaseDSN = envDataBaseDSN
	}

	if envAuditFile := os.Getenv("AUDIT_FILE"); envAuditFile != "" {
		cfg.AuditFile = envAuditFile
	}
	if envAuditURL := os.Getenv("AUDIT_URL"); envAuditURL != "" {
		cfg.AuditURL = envAuditURL
	}

	return cfg
}
