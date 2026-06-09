package main

import (
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/router"
	"github.com/stretchr/testify/assert"
)

func Test_main(t *testing.T) {
	// запускаем тестовый сервер, будет выбран первый свободный порт
	srv := httptest.NewServer(router.SetupRouter())
	config.ParseFlags()
	// останавливаем сервер после завершения теста
	defer srv.Close()

	//save site for reading in tests
	request := resty.New().R()
	request.Method = http.MethodPost
	request.Body = "https://yandex.ru"
	request.URL = srv.URL + "/"
	resp, err := request.Send()
	assert.NoError(t, err, "error making HTTP request for store site")
	body := string(resp.Body())
	alias := body[len(body)-8:]
	log.Println(alias)

	tests := []struct {
		name             string
		method           string
		path             string
		body             string
		expectedCode     int
		expectedLocation string
		expectedBody     string
	}{
		{
			name:         "Short new site",
			method:       http.MethodPost,
			path:         "/",
			body:         "ya.ru",
			expectedCode: http.StatusCreated,
		},
		{
			name:             "Get stored site",
			method:           http.MethodGet,
			path:             "/" + alias,
			expectedCode:     http.StatusTemporaryRedirect,
			expectedLocation: "https://yandex.ru",
		},
		{
			name:         "Get not existing site",
			method:       http.MethodGet,
			path:         "/123",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Store allready stored",
			method:       http.MethodPost,
			path:         "/",
			body:         "https://yandex.ru",
			expectedCode: http.StatusCreated,
			expectedBody: "http://localhost:8080/" + alias,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//disable resty redirects
			req := resty.New().SetRedirectPolicy(resty.NoRedirectPolicy()).R()
			req.Method = tt.method
			req.Body = tt.body
			req.URL = srv.URL + tt.path
			log.Println("Performing resty request to URL", req.URL)

			resp, err := req.Send()
			if !errors.Is(err, resty.ErrAutoRedirectDisabled) {
				assert.NoError(t, err, "error making HTTP request")
			}

			assert.Equal(t, tt.expectedCode, resp.StatusCode(), "Response code didn't match expected")

			// проверяем корректность полученного в заголовке редиректа
			if tt.expectedLocation != "" {
				assert.Equal(t, tt.expectedLocation, string(resp.Header().Get("Location")))
			}

			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, string(resp.Body()))
			}
		})
	}
}
