package benchmark

import (
	"net/http/httptest"
	"strconv"
	"testing"
)

// Benchmark_E2E_Post_Text тестирует хэппи-пас (201 Created) текстового хендлера (POST /)
func Benchmark_E2E_Post_Text(b *testing.B) {
	// Инициализируем изолированное стерильное окружение для этого теста
	env := initBenchEnv(b)

	// Выделяем базовый буфер под уникальный URL: "https://yandex.ru"
	// 18 байт под префикс + 20 байт под максимальный uint64 (всего 38 байт емкости)
	basePayload := make([]byte, 18, 38)
	copy(basePayload, "https://yandex.ru")

	var counter uint64
	res := httptest.NewRecorder()

	b.ResetTimer()
	for b.Loop() {
		counter++
		// Быстрый сброс длины слайса до базового префикса
		payload := basePayload[:18]
		// Дописываем число в конец байтового слайса БЕЗ выделения памяти (0 allocs)
		payload = strconv.AppendUint(payload, counter, 10)

		resetRecorder(res)

		executePostRequest(b, env.Ctx, env.Router, "/", "text/plain", "", "", payload, res, env.Cookie)
	}
}

// Benchmark_E2E_Post_JSON тестирует хэппи-пас (201 Created) REST API хендлера (POST /api/shorten)
func Benchmark_E2E_Post_JSON(b *testing.B) {
	// Инициализируем изолированное стерильное окружение для этого теста
	env := initBenchEnv(b)

	// Шаблон JSON: {"url": "https://yandex.ru"}
	prefix := []byte(`{"url": "https://yandex.ru`)
	suffix := []byte(`"}`)

	// Буфер под итоговый JSON с запасом под длинные числа для 0 аллокаций
	payloadBuf := make([]byte, 0, 64)
	var counter uint64
	res := httptest.NewRecorder()

	b.ResetTimer()
	for b.Loop() {
		counter++

		// Собираем валидный JSON в буфере заново на каждой итерации без аллокаций в куче
		payloadBuf = payloadBuf[:0]
		payloadBuf = append(payloadBuf, prefix...)
		payloadBuf = strconv.AppendUint(payloadBuf, counter, 10)
		payloadBuf = append(payloadBuf, suffix...)

		resetRecorder(res)

		executePostRequest(b, env.Ctx, env.Router, "/api/shorten", "application/json", "", "", payloadBuf, res, env.Cookie)
	}
}
