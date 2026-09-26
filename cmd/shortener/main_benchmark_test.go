package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/router"
)

// Benchmark_E2E_Post_Text замеряет чистую скорость работы текстового хендлера.
func Benchmark_E2E_Post_Text(b *testing.B) {
	ctx := context.Background()
	cfg := &config.Config{
		ServerAddress: ":8080",
		ShortBaseUrl:  "http://localhost:8080/",
		StoragePath:   "bench_storage_text.json",
	}

	appRouter, repo, _ := router.SetupRouter(ctx, cfg)

	b.Cleanup(func() {
		_ = repo.Close()
		_ = os.Remove("bench_storage_text.json")
	})

	payload := []byte("https://yandex.ru")

	b.ResetTimer()

	for b.Loop() {
		// Безопасно конструируем запрос с обязательной проверкой ошибки компиляции
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(payload))
		if err != nil {
			b.Fatalf("критическая ошибка сборки запроса: %v", err)
		}

		req.Header.Set("Content-Type", "text/plain")
		res := httptest.NewRecorder()

		appRouter.ServeHTTP(res, req)
	}
}

// Benchmark_E2E_Post_JSON оценивает накладные расходы REST API хендлера (/api/shorten).
func Benchmark_E2E_Post_JSON(b *testing.B) {
	ctx := context.Background()
	cfg := &config.Config{
		ServerAddress: ":8080",
		ShortBaseUrl:  "http://localhost:8080/",
		StoragePath:   "bench_storage_json.json",
	}

	appRouter, repo, _ := router.SetupRouter(ctx, cfg)

	b.Cleanup(func() {
		_ = repo.Close()
		_ = os.Remove("bench_storage_json.json")
	})

	payload := []byte(`{"url": "https://yandex.ru"}`)

	b.ResetTimer()

	for b.Loop() {
		// Безопасный вызов метода с фиксацией и проверкой ошибки
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/shorten", bytes.NewReader(payload))
		if err != nil {
			b.Fatalf("критическая ошибка сборки JSON запроса: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()

		appRouter.ServeHTTP(res, req)
	}
}
