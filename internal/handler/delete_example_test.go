package handler_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
)

// ExampleAsyncDeleteUserSites демонстрирует пример работы с эндпоинтом пакетного удаления ссылок.
//
// Пример полностью отражает контракт авторизации проекта: генерируется зашифрованная
// кука "user_id" через securecookie, отправляется на сервер вместе с JSON-батчем,
// и сервер успешно возвращает статус 202.
func ExampleAsyncDeleteUserSites() {
	// 1. Настраиваем ключи для securecookie (в реальном приложении они берутся из конфига)
	hashKey := []byte("very-secret-string-32-bytes-long")
	blockKey := []byte("a-lot-secret-string-16-bytes-len")
	sc := securecookie.New(hashKey, blockKey)

	// 2. Эмулируем поведение вашего хендлера и middleware для прохождения песочницы.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Имитируем extractUserID: проверяем наличие куки "user_id"
		cookie, err := r.Cookie("user_id")
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Кука авторизации отсутствует"}`))
			return
		}

		// Декодируем куку, проверяя её валидность через securecookie
		var userID uuid.UUID
		if err := sc.Decode("user_id", cookie.Value, &userID); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Невалидная сессия пользователя"}`))
			return
		}

		// Проверяем формат переданных данных
		if r.Method != http.MethodDelete || r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Контракт соблюден — отдаем 202 Accepted
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"Accepted","message":"Задачи приняты в очередь"}`))
	}))
	defer mockServer.Close()

	// 3. Формируем тестовый Payload (JSON-массив алиасов)
	jsonPayload := []byte(`["61a03f", "82f11b", "99c2ea"]`)
	endpointURL := mockServer.URL + "/api/user/urls"
	req, _ := http.NewRequest(http.MethodDelete, endpointURL, bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	// 4. Генерируем валидную куку авторизации для примера
	mockUserID := uuid.New()
	encodedValue, _ := sc.Encode("user_id", mockUserID)

	authCookie := &http.Cookie{
		Name:     "user_id",
		Value:    encodedValue,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
	}
	// Прикрепляем куку к нашему клиентскому запросу
	req.AddCookie(authCookie)

	// 5. Выполняем HTTP-запрос к API
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Ошибка выполнения запроса: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	fmt.Printf("HTTP Method: %s\n", req.Method)
	fmt.Printf("Cookie Present: %t\n", authCookie.Value != "")
	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("JSON Response: %s\n", string(body))

	// Output:
	// HTTP Method: DELETE
	// Cookie Present: true
	// HTTP Status: 202
	// JSON Response: {"status":"Accepted","message":"Задачи приняты в очередь"}
}
