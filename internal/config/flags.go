package config

import (
	"flag"
	"net/url"
	"os"
	"strings"
)

// Config содержит глобальные настройки конфигурации приложения.
type Config struct {
	ServerAddress string
	ShortBaseUrl  *url.URL
	StoragePath   string
	DataBaseDSN   string
	AuditFile     string
	AuditURL      string
}

// ParseFlags парсит переданные флаги командной строки и переменные окружения,
// а также выполняет нормализацию и валидацию критичных сетевых адресов.
func ParseFlags() *Config {
	cfg := &Config{}
	var rawShortBaseURL string
	flag.StringVar(&cfg.ServerAddress, "a", ":8080", "адрес запуска HTTP-сервера")
	flag.StringVar(&rawShortBaseURL, "b", "http://localhost:8080/", "базовый адрес результирующего сокращённого URL")
	flag.StringVar(&cfg.StoragePath, "f", "storage.json", "путь к файлу для сохранения сайтов")
	flag.StringVar(&cfg.DataBaseDSN, "d", "", "параметры подключения к БД")
	flag.StringVar(&cfg.AuditFile, "audit-file", "audit.json", "путь к файлу-приёмнику логов аудита")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "URL удаленного сервера-приёмника логов аудита")
	flag.Parse()

	// Переопределяем из переменных окружения, если они заданы
	if envServerAddress := os.Getenv("SERVER_ADDRESS"); envServerAddress != "" {
		cfg.ServerAddress = envServerAddress
	}
	if envBaseUrl := os.Getenv("BASE_URL"); envBaseUrl != "" {
		rawShortBaseURL = envBaseUrl
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

	// Нормализация и автоподстановка схемы
	cfg.AuditURL = normalizeURL(cfg.AuditURL)

	var err error

	cfg.ShortBaseUrl, err = url.Parse(normalizeURL(rawShortBaseURL))
	if err != nil {
		panic("invalid base URL: " + err.Error())
	}

	return cfg
}

// normalizeURL автоматически добавляет http:// к сырым адресам (например, "localhost:8080"),
// если пользователь забыл указать протокол схемы при запуске.
func normalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	// Если адрес передан без протокола, url.Parse воспримет его некорректно.
	// Проверяем явное наличие разделителя схемы.
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		// Автоматически подставляем дефолтный http://
		rawURL = "http://" + rawURL
	}

	// Гарантируем отсутствие слэша на конце для будущих хэндлеров
	rawURL = strings.TrimSuffix(rawURL, "/")

	return rawURL
}
