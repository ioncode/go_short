package pkg

import (
	"compress/gzip"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/ioncode/go_short/internal/logger"
	"go.uber.org/zap"
)

var writerPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

var readerPool = sync.Pool{
	New: func() any {
		return &gzip.Reader{}
	},
}

// compressWriter реализует интерфейс http.ResponseWriter и позволяет прозрачно для сервера
// сжимать передаваемые данные и выставлять правильные HTTP-заголовки
type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	zw := writerPool.Get().(*gzip.Writer)
	zw.Reset(w)
	return &compressWriter{
		w:  w,
		zw: zw,
	}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		c.w.Header().Set("Content-Encoding", "gzip")
	}
	c.w.WriteHeader(statusCode)
}

// Close закрывает gzip.Writer и досылает все данные из буфера.
func (c *compressWriter) Close() error {
	err := c.zw.Close()
	c.zw.Reset(io.Discard)
	writerPool.Put(c.zw)
	return err
}

// compressReader реализует интерфейс io.ReadCloser и позволяет прозрачно для сервера
// декомпрессировать получаемые от клиента данные
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

// newCompressReader извлекает готовый gzip.Reader из пула памяти readerPool
// и инициализирует его под текущий io.ReadCloser (тело запроса) с помощью Reset.
// Это полностью предотвращает аллокации flate.NewReader в куче на каждый входящий запрос.
func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	// 1. Достаем свободный декомпрессор из sync.Pool
	zr := readerPool.Get().(*gzip.Reader)

	// 2. Сбрасываем его состояние и прикладываем к текущему входящему потоку.
	// Если это первый запуск структуры из пула (ее внутренний ридер nil),
	// gzip.NewReader(r) инициализирует ее, иначе Reset подменит сокет за 0 аллокаций.
	err := zr.Reset(r)
	if err != nil {
		// В случае ошибки (например, битый заголовок gzip) возвращаем ридер обратно в пул,
		// чтобы избежать утечки ресурсов из пула памяти.
		readerPool.Put(zr)
		return nil, err
	}

	return &compressReader{
		r:  r,
		zr: zr,
	}, nil
}

func (c compressReader) Read(p []byte) (n int, err error) {
	return c.zr.Read(p)
}

// Close закрывает входящие сетевые потоки и возвращает декомпрессор в пул.
func (c *compressReader) Close() error {
	// 1. Закрываем оригинальный сетевой поток тела запроса (req.Body)
	err := c.r.Close()

	// 2. Закрываем внутренний декомпрессор gzip
	if zrErr := c.zr.Close(); zrErr != nil {
		err = errors.Join(err, zrErr)
	}

	// 3. Зануляем ссылку на входящий поток внутри gzip.Reader,
	// чтобы разорвать связь с закрытым сокетом и дать GC очистить его метаданные.
	_ = c.zr.Reset(io.NopCloser(strings.NewReader("")))

	// 4. Возвращаем чистый декомпрессор в пул для использования другими горутинами
	readerPool.Put(c.zr)

	return err
}

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// по умолчанию устанавливаем оригинальный http.ResponseWriter как тот,
		// который будем передавать следующей функции
		ow := w

		// проверяем, что клиент умеет получать от сервера сжатые данные в формате gzip
		acceptEncoding := r.Header.Get("Accept-Encoding")
		contentType := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			logger.Log.Debug("Error parsing mediatype from content type",
				zap.String("content_type", contentType),
			)
			//http.Error(w, "Invalid Content-Type header", http.StatusBadRequest)
			//return
		}

		supportsGzip := strings.Contains(acceptEncoding, "gzip") && (mediaType == "application/json" || mediaType == "text/html")
		if supportsGzip {
			// оборачиваем оригинальный http.ResponseWriter новым с поддержкой сжатия
			cw := newCompressWriter(w)
			//устанавливаем заголовок сжатого ответа
			cw.w.Header().Set("Content-Encoding", "gzip")
			// меняем оригинальный http.ResponseWriter на новый
			ow = cw
			// не забываем отправить клиенту все сжатые данные после завершения middleware
			defer cw.Close()
		}

		// проверяем, что клиент отправил серверу сжатые данные в формате gzip
		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			// оборачиваем тело запроса в io.Reader с поддержкой декомпрессии
			cr, err := newCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				logger.Log.Error("Error creating gzip compressor",
					zap.Error(err),
				)
				return
			}
			// меняем тело запроса на новое
			r.Body = cr
			defer cr.Close()
		}

		// передаём управление хендлеру
		next.ServeHTTP(ow, r)
	})
}
