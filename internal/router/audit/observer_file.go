package audit

import (
	"context"
	"os"
	"sync"

	json "github.com/goccy/go-json"
	"go.uber.org/zap"
)

// FileObserver записывает события аудита в файл методом потокового JSON-кодирования
// и логирует внутренние ошибки записи через zap.Logger.
type FileObserver struct {
	mu      sync.Mutex
	file    *os.File
	encoder *json.Encoder
	logger  *zap.Logger
}

// NewFileObserver открывает файл, инициализирует JSON-энкодер и принимает логгер как зависимость.
func NewFileObserver(path string, logger *zap.Logger) (*FileObserver, error) {
	// Открываем файл в режиме добавления (O_APPEND) или создаем его (O_CREATE)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	return &FileObserver{
		file:    file,
		encoder: json.NewEncoder(file),
		// Создаем изолированную именованную область логов для этого наблюдателя
		logger: logger.Named("audit_file_observer"),
	}, nil
}

// Name возвращает имя подписчика.
func (f *FileObserver) Name() string {
	return "Файловый приемник"
}

// OnRequest асинхронно кодирует событие прямо в дескриптор файла.
func (f *FileObserver) OnRequest(ctx context.Context, e Event) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Потоковое кодирование: пишем напрямую в дескриптор файла.
	if err := f.encoder.Encode(e); err != nil {
		// Записываем лог при ошибке записи или кодирования
		f.logger.Error("Не удалось записать событие в файл аудита",
			zap.Error(err),
			zap.Int64("event_ts", e.TS),
			zap.String("action", string(e.Action)),
			zap.String("path", e.Path),
		)
	}
}

// Close сбрасывает кэш операционной системы на диск и закрывает файл.
func (f *FileObserver) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.file.Sync(); err != nil {
		_ = f.file.Close()
		return err
	}

	return f.file.Close()
}
