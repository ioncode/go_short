package handler_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
)

// ExampleGetUserSites демонстрирует пример работы с эндпоинтом получения ссылок пользователя.
//
// Эндпоинт возвращает статус 200 OK и JSON-массив со всеми сокращенными ссылками,
// принадлежащими авторизованному пользователю. Если у пользователя нет ссылок, вернется статус 204 No Content.
func ExampleGetUserSites() {
	// 1. Настраиваем базовый URL для формирования коротких ссылок
	shortBaseURL, _ := url.Parse("http://localhost:8080/")

	// 2. Эмулируем поведение эндпоинта GET /api/user/urls для прохождения песочницы godoc
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Описываем структуру ответа локально в полном соответствии с моделью model.UserSitesResponseItem
		type userSitesResponseItem struct {
			Alias string `json:"short_url"`
			URL   string `json:"original_url"`
		}

		// Имитируем данные из базы данных
		records := []userSitesResponseItem{
			{Alias: "61a03f", URL: "https://google.com"},
			{Alias: "82f11b", URL: "https://yandex.ru"},
		}

		// Моделируем логику мутации алиасов в полноценные ссылки
		u := *shortBaseURL
		for i := range records {
			u.Path = records[i].Alias
			records[i].Alias = u.String()
		}

		// Отдаем JSON стандартными средствами для стабильности в godoc sandbox
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(&records)
	}))
	defer mockServer.Close()

	// 3. Выполняем HTTP-запрос к эмулируемому API
	endpointURL := mockServer.URL + "/api/user/urls"
	req, _ := http.NewRequest(http.MethodGet, endpointURL, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Ошибка выполнения запроса: %v\n", err)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, _ := io.ReadAll(resp.Body)

	// 4. Выводим результаты для отображения в документации
	fmt.Printf("HTTP Method: %s\n", req.Method)
	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("JSON Response: %s", string(body))

	// Output:
	// HTTP Method: GET
	// HTTP Status: 200
	// Content-Type: application/json
	// JSON Response: [{"short_url":"http://localhost:8080/61a03f","original_url":"https://google.com"},{"short_url":"http://localhost:8080/82f11b","original_url":"https://yandex.ru"}]
}
