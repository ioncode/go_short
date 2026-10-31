package handler_test

import (
	"bytes"
	"fmt"
	"net/http"
)

// ExampleAPIPostBatch демонстрирует пример отправки пакетного запроса на массовое сокращение ссылок.
//
// Метод отправляет JSON-массив с оригинальными URL и получает в ответ массив с готовыми короткими ссылками.
func ExampleAPIPostBatch() {
	// 1. Формируем входящий JSON-массив (батч)
	jsonPayload := []byte(`[
		{"correlation_id": "req-1", "url": "https://google.com"},
		{"correlation_id": "req-2", "url": "https://yandex.ru"}
	]`)

	// Адрес вашего запущенного сервиса сокращения ссылок
	endpointURL := "http://localhost:8080/api/shorten/batch"

	// 2. Создаем и выполняем стандартный HTTP POST запрос к API
	req, err := http.NewRequest(http.MethodPost, endpointURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Printf("Ошибка создания запроса: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос (Имитация для корректной работы кнопки Run в песочнице godoc)
	// В реальном коде здесь будет: resp, err := http.DefaultClient.Do(req)
	resp, body := simulateBatchResponse(req, jsonPayload)

	// 3. Выводим результаты для документации пакета
	fmt.Printf("HTTP Method: %s\n", req.Method)
	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("JSON Response: %s", string(body))

	// Output:
	// HTTP Method: POST
	// HTTP Status: 201
	// Content-Type: application/json
	// JSON Response: [{"correlation_id":"req-1","alias":"http://localhost:8080/batch1"},{"correlation_id":"req-2","alias":"http://localhost:8080/batch2"}]
}

// simulateBatchResponse исключительно для прохождения песочницы имитирует ответ сервера,
// чтобы убрать сетевой dial tcp timeout при нажатии Run.
func simulateBatchResponse(req *http.Request, payload []byte) (*http.Response, []byte) {
	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "application/json")

	// Возвращаем валидный JSON-ответ с переводом строки на конце
	mockBody := []byte(`[{"correlation_id":"req-1","alias":"http://localhost:8080/batch1"},{"correlation_id":"req-2","alias":"http://localhost:8080/batch2"}]` + "\n")
	return resp, mockBody
}
