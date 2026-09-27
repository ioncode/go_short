package benchmark

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/router"
	"github.com/ioncode/go_short/internal/service"
)

// Глобальный синк для предотвращения оптимизаций компилятора
var benchSink *httptest.ResponseRecorder

// Глобальные переменные окружения бенчмарков
var (
	//  переменная для хранения перехваченной валидной куки авторизации
	authCookie  *http.Cookie
	benchUserID string // Сохраняем чистый UUID здесь один раз при старте!
)

// initBenchEnv создает изолированный роутер и перехватывает валидную сессию
func initBenchEnv() (context.Context, http.Handler, service.SiteRepository) {
	ctx := context.Background()
	cfg := &config.Config{
		ServerAddress: ":8080",
		ShortBaseUrl:  "http://localhost:8080/",
		StoragePath:   "", // Только RAM
	}

	appRouter, repo, _ := router.SetupRouter(ctx, cfg)

	if authCookie == nil {
		// 1. Делаем один прогревочный POST-запрос
		warmupURL := "https://baseline-warmup.com"
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader([]byte(warmupURL)))
		req.Header.Set("Content-Type", "text/plain")

		res := httptest.NewRecorder()
		appRouter.ServeHTTP(res, req)

		// 2. Перехватываем зашифрованную куку авторизации для b.Loop
		for _, c := range res.Result().Cookies() {
			if c.Name == "user_id" {
				authCookie = c
				break
			}
		}

		// 3. Мгновенно достаем расшифрованный UserId из репозитория по оригинальному URL
		if site, err := repo.GetByUrl(model.Url(warmupURL)); err == nil {
			benchUserID = site.UserId
		}
	}

	return ctx, appRouter, repo
}

// generateBatchJSON собирает JSON батча без аллокаций в куче
func generateBatchJSON(buf []byte, counter uint64) []byte {
	p1 := []byte(`[{"correlation_id":"id1_`)
	p2 := []byte(`","original_url":"https://yandex.ru_`)
	p3 := []byte(`"},{"correlation_id":"id2_`)
	p4 := []byte(`","original_url":"https://yandex.ru_`)
	p5 := []byte(`"}]`)

	buf = buf[:0]
	buf = append(buf, p1...)
	buf = strconv.AppendUint(buf, counter, 10)
	buf = append(buf, p2...)
	buf = strconv.AppendUint(buf, counter, 10)
	buf = append(buf, p3...)
	buf = strconv.AppendUint(buf, counter, 10)
	buf = append(buf, p4...)
	buf = strconv.AppendUint(buf, counter, 10)
	buf = append(buf, p5...)
	return buf
}

// resetRecorder безопасно очищает рекордер, не ломая внутреннюю HTTP-семантику пакета httptest
func resetRecorder(res *httptest.ResponseRecorder) {
	res.Code = 0
	res.Body.Reset()
	res.HeaderMap = make(http.Header)
}

// executePostRequest — строго типизированная функция для отправки POST-запросов и их валидации
func executePostRequest(
	b *testing.B,
	ctx context.Context,
	router http.Handler,
	path string,
	contentType string,
	contentEncoding string,
	acceptEncoding string,
	payload []byte,
	res *httptest.ResponseRecorder,
) {
	bodyReader := bytes.NewReader(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bodyReader)
	if err != nil {
		b.Fatalf("критическая ошибка создания запроса: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
	}
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}

	// Подставляем честно подписанную куку сессии
	if authCookie != nil {
		req.AddCookie(authCookie)
	}

	router.ServeHTTP(res, req)

	respResult := res.Result()
	defer respResult.Body.Close()

	if respResult.StatusCode != http.StatusCreated {
		b.Fatalf("Ожидался статус 201, получен %d: %s", respResult.StatusCode, res.Body.String())
	}

	if acceptEncoding != "" {
		if _, err := io.Copy(io.Discard, res.Body); err != nil {
			b.Fatalf("ошибка вычитки gzipped-ответа: %v", err)
		}
	}

	benchSink = res
}
