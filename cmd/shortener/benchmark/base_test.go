package benchmark

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/router"
	"github.com/ioncode/go_short/internal/service"
)

var benchSink *httptest.ResponseRecorder

func init() {
	log.SetOutput(io.Discard)
}

// Структура для возврата полной изоляции окружения
type BenchEnv struct {
	Ctx    context.Context
	Router http.Handler
	Repo   service.SiteRepository
	Cookie *http.Cookie
	UserID string
}

func initBenchEnv(b *testing.B) BenchEnv {
	ctx := context.Background()

	shortBaseUrl, _ := url.Parse("http://localhost:8080/")
	cfg := &config.Config{
		ServerAddress: ":8080",
		ShortBaseUrl:  shortBaseUrl,
		StoragePath:   "", // Чистая RAM
	}

	appRouter, repo, _ := router.SetupRouter(ctx, cfg)

	// Прогревочный запрос для перехвата сессии инстанса роутера
	warmupURL := "https://baseline-warmup.com"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader([]byte(warmupURL)))
	if err != nil {
		b.Fatalf("ошибка прогревочного запроса: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	res := httptest.NewRecorder()
	appRouter.ServeHTTP(res, req)

	var cookie *http.Cookie
	for _, c := range res.Result().Cookies() {
		if c.Name == "user_id" {
			cookie = c
			break
		}
	}

	var userID string
	if site, err := repo.GetByUrl(model.Url(warmupURL)); err == nil {
		userID = site.UserId
	}

	return BenchEnv{
		Ctx:    ctx,
		Router: appRouter,
		Repo:   repo,
		Cookie: cookie,
		UserID: userID,
	}
}

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

func resetRecorder(res *httptest.ResponseRecorder) {
	res.Code = 0
	res.Body.Reset()
	res.HeaderMap = make(http.Header)
}

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
	cookie *http.Cookie,
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

	if cookie != nil {
		req.AddCookie(cookie)
	}

	router.ServeHTTP(res, req)

	respResult := res.Result()
	respResult.Body.Close()

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
