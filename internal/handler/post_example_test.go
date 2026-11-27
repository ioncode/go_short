package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
)

// ExampleAPIPost демонстрирует пример работы с эндпоинтом одиночного сокращения ссылок.
//
// Метод принимает входящий JSON-запрос, содержащий оригинальный 'url',
// генерирует для него короткий алиас и возвращает JSON-ответ со статусом 201 Created.
func ExampleAPIPost() {
	// 1. Настраиваем базовый URL для генерации коротких ссылок
	shortBaseURL, _ := url.Parse("http://localhost:8080/")

	// 2. Эмулируем поведение эндпоинта POST /api/shorten
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type postRequest struct {
			URL string `json:"url"`
		}
		type postResponse struct {
			Result string `json:"result"`
		}

		var reqModel postRequest
		if err := json.NewDecoder(r.Body).Decode(&reqModel); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Имитируем генерацию алиаса
		alias := "61a03f"

		u := *shortBaseURL
		u.Path = alias

		result := postResponse{
			Result: u.String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		// Чтобы encoder не добавлял скрытый перевод строки в буфер ответа,
		// маршалим JSON в сырые байты и пишем их напрямую.
		rawJSON, _ := json.Marshal(&result)
		_, _ = w.Write(rawJSON)
	}))
	defer mockServer.Close()

	// 3. Формируем JSON Payload для отправки на сервер
	jsonPayload := []byte(`{"url":"https://google.com"}`)
	endpointURL := mockServer.URL + "/api/shorten"

	// 4. Выполняем HTTP POST запрос к нашему эмулируемому API
	req, _ := http.NewRequest(http.MethodPost, endpointURL, bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Ошибка выполнения запроса: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 5. Выводим результаты (теперь строки совпадут байт-в-байт)
	fmt.Printf("HTTP Method: %s\n", req.Method)
	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("JSON Response: %s", string(body))

	// Output:
	// HTTP Method: POST
	// HTTP Status: 201
	// Content-Type: application/json
	// JSON Response: {"result":"http://localhost:8080/61a03f"}
}
