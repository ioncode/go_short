package benchmark

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// generateDeleteJSON собирает JSON-массив алиасов для удаления без аллокаций в куче
func generateDeleteJSON(buf []byte, counter uint64) []byte {
	// Формируем JSON вида: ["del_1","del_2"]
	p1 := []byte(`["del_`)
	p2 := []byte(`","del_`)
	p3 := []byte(`"]`)

	buf = buf[:0] // Быстрый сброс длины с сохранением емкости (capacity)
	buf = append(buf, p1...)
	buf = strconv.AppendUint(buf, counter, 10)
	buf = append(buf, p2...)
	buf = strconv.AppendUint(buf, counter, 10)
	buf = append(buf, p3...)
	return buf
}

// Benchmark_E2E_DeleteUserSites замеряет скорость постановки задач на асинхронное удаление
func Benchmark_E2E_DeleteUserSites(b *testing.B) {
	ctx, appRouter, _ := initBenchEnv()

	// Выделяем переиспользуемые буферы под JSON и Reader тела запроса
	rawBuf := make([]byte, 0, 128)
	bodyReader := bytes.NewReader(nil)

	res := httptest.NewRecorder()

	// Вводим локальный счетчик с правильным типом uint64 для динамического Payload
	var counter uint64

	b.ResetTimer()
	for b.Loop() {
		counter++

		// 1. Быстро сбрасываем и очищаем рекордер ответа
		resetRecorder(res)

		// 2. Генерируем уникальный JSON-массив
		rawBuf = generateDeleteJSON(rawBuf, counter)
		bodyReader.Reset(rawBuf)

		// 3. Конструируем запрос на удаление
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "/api/user/urls", bodyReader)
		if err != nil {
			b.Fatalf("критическая ошибка создания запроса: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// 4. Подставляем валидную перехваченную куку авторизации
		if authCookie != nil {
			req.AddCookie(authCookie)
		}

		// 5. Отправляем запрос в роутер
		appRouter.ServeHTTP(res, req)

		// 6. Достаем результирующий статус-код сервера
		respResult := res.Result()
		respResult.Body.Close()

		// Хендлер по спецификации должен возвращать статус 202 Accepted
		if respResult.StatusCode != http.StatusAccepted {
			b.Fatalf("Ожидался статус 202 Accepted, получен %d. Ответ: %s", respResult.StatusCode, res.Body.String())
		}

		benchSink = res
	}
}
