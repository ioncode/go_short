package benchmark

import (
	"net/http/httptest"
	"strconv"
	"testing"
)

func Benchmark_E2E_Post_Text(b *testing.B) {
	ctx, appRouter, _ := initBenchEnv()
	basePayload := make([]byte, 18, 38)
	copy(basePayload, "https://yandex.ru")
	var counter uint64

	res := httptest.NewRecorder()

	for b.Loop() {
		counter++
		payload := basePayload[:18]
		payload = strconv.AppendUint(payload, counter, 10)

		resetRecorder(res)

		executePostRequest(b, ctx, appRouter, "/", "text/plain", "", "", payload, res)
	}
}

func Benchmark_E2E_Post_JSON(b *testing.B) {
	ctx, appRouter, _ := initBenchEnv()
	prefix, suffix := []byte(`{"url": "https://yandex.ru`), []byte(`"}`)
	payloadBuf := make([]byte, 0, 64)
	var counter uint64

	res := httptest.NewRecorder()

	for b.Loop() {
		counter++
		payloadBuf = payloadBuf[:0]
		payloadBuf = append(payloadBuf, prefix...)
		payloadBuf = strconv.AppendUint(payloadBuf, counter, 10)
		payloadBuf = append(payloadBuf, suffix...)

		resetRecorder(res)

		executePostRequest(b, ctx, appRouter, "/api/shorten", "application/json", "", "", payloadBuf, res)
	}
}
