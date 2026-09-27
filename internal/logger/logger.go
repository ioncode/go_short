package logger

import (
	"errors"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log будет доступен всему коду как синглтон.
// Никакой код навыка, кроме функции Initialize, не должен модифицировать эту переменную.
// По умолчанию установлен no-op-логер, который не выводит никаких сообщений.
var Log *zap.Logger = zap.NewNop()

// Переменная для контроля буфера асинхронного логирования
var bufferedSyncer *zapcore.BufferedWriteSyncer

// Initialize инициализирует синглтон логера с необходимым уровнем логирования.
func Initialize(level string) error {
	// преобразуем текстовый уровень логирования в zap.AtomicLevel
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return err
	}
	// создаём новую конфигурацию логера
	cfg := zap.NewProductionConfig()
	// устанавливаем уровень
	cfg.Level = lvl
	// === НАЧАЛО ОПТИМИЗАЦИИ ДЛЯ ПРОДАКШЕНА (АСИНХРОННОСТЬ) ===
	// 1. Берем стандартный вывод (консоль)
	stderrSyncer := zapcore.Lock(os.Stderr)

	// 2. Оборачиваем его в асинхронный буфер объемом 256 КБ
	// Логи будут копироваться в RAM мгновенно, не блокируя HTTP-хендлеры!
	bufferedSyncer = &zapcore.BufferedWriteSyncer{
		WS:            stderrSyncer,
		Size:          256 * 1024,  // 256 KB буфер
		FlushInterval: time.Second, // Сбрасывать на диск гарантированно раз в секунду
	}

	// 3. Собираем ядро логгера с асинхронным буфером
	encoder := zapcore.NewJSONEncoder(cfg.EncoderConfig)
	core := zapcore.NewCore(encoder, bufferedSyncer, lvl)

	// 4. Строим финальный логгер
	zl := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	// === КОНЕЦ ОПТИМИЗАЦИИ ===
	// устанавливаем синглтон
	Log = zl
	return nil
}

// Sync вызывается при остановке приложения (Graceful Shutdown) в main.go,
// чтобы принудительно вытолкнуть остатки логов из буфера.
func Sync() error {
	var err error
	if bufferedSyncer != nil {
		err = bufferedSyncer.Sync() // Сбрасываем асинхронный буфер байт
	}
	if Log != nil {
		err = errors.Join(err, Log.Sync()) // Сбрасываем сам zap
	}
	return err
}

// RequestLogger — middleware-логер для входящих HTTP-запросов.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Log.Core().Enabled(zapcore.InfoLevel) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		Log.Info("got incoming HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Duration("duration", duration),
		)
	})
}

type (
	// берём структуру для хранения сведений об ответе
	responseData struct {
		status int
		size   int
	}

	// добавляем реализацию http.ResponseWriter
	loggingResponseWriter struct {
		http.ResponseWriter // встраиваем оригинальный http.ResponseWriter
		responseData        *responseData
	}
)

func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	// записываем ответ, используя оригинальный http.ResponseWriter
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size // захватываем размер
	return size, err
}

func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	// записываем код статуса, используя оригинальный http.ResponseWriter
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode // захватываем код статуса
}

// ResponseLogger — middleware-логер для исходящих HTTP-ответов.
func ResponseLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Log.Core().Enabled(zapcore.InfoLevel) {
			next.ServeHTTP(w, r)
			return
		}
		responseData := &responseData{
			status: 0,
			size:   0,
		}
		lw := loggingResponseWriter{
			ResponseWriter: w, // встраиваем оригинальный http.ResponseWriter
			responseData:   responseData,
		}
		next.ServeHTTP(&lw, r)
		Log.Info("sending response",
			zap.Int("status", responseData.status),
			zap.Int("size", responseData.size),
		)
	})
}
