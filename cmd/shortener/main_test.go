package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/router"
	"github.com/stretchr/testify/assert"
)

func Test_main(t *testing.T) {
	// запускаем тестовый сервер, будет выбран первый свободный порт

	router, repo := router.SetupRouter(config.Config{
		ServerAddress: ":8080",
		ShortBaseUrl:  "http://localhost:8080/",
		StoragePath:   "test_storage.json",
	})
	srv := httptest.NewServer(router)

	t.Cleanup(func() {
		// останавливаем сервер после завершения теста
		srv.Close()
		//закрываем репозиторий
		repo.Close()
		//удаляем файл хранения
		os.Remove("test_storage.json")
	})

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
		name                    string
		method                  string
		path                    string
		body                    string
		expectedCode            int
		expectedLocation        string
		expectedBody            string
		contentType             string
		expectedContentType     string
		contentEncoding         string
		expectedContentEncoding string
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
			name:                    "Store allready stored VIA REST",
			method:                  http.MethodPost,
			path:                    "/api/shorten",
			body:                    `{"url": "Https://yandex.Ru"}`,
			contentType:             "application/json",
			expectedCode:            http.StatusConflict,
			expectedBody:            "{\"result\":\"http://localhost:8080/" + alias + "\"}\n",
			expectedContentEncoding: "gzip",
		},
		{
			name:                    "Short site via JSON REST API",
			method:                  http.MethodPost,
			path:                    "/api/shorten",
			body:                    `{"url": "Https://practicum.yandex.Ru"}`,
			expectedCode:            http.StatusCreated,
			contentType:             "application/json",
			expectedContentType:     "application/json",
			contentEncoding:         "gzip",
			expectedContentEncoding: "gzip",
		},
		{
			name:                "Payload error in JSON REST API",
			method:              http.MethodPost,
			path:                "/api/shorten",
			body:                `{"badkey": "Https://practicum.yandex.Ru"}`,
			expectedCode:        http.StatusBadRequest,
			contentType:         "application/json",
			expectedContentType: "application/json",
		},
		{
			name:                    "Batch Store allready stored",
			method:                  http.MethodPost,
			path:                    "/api/shorten/batch",
			body:                    `[{"original_url": "Https://yandex.Ru","correlation_id": "uuid1"}, {"original_url": "Https://yandex.Ru","correlation_id": "uuid2"}]`,
			contentType:             "application/json",
			expectedCode:            http.StatusCreated,
			expectedBody:            "[{\"short_url\":\"http://localhost:8080/" + alias + "\",\"correlation_id\":\"uuid1\"},{\"short_url\":\"http://localhost:8080/" + alias + "\",\"correlation_id\":\"uuid2\"}]\n",
			expectedContentEncoding: "gzip",
		},
		{
			name:                    "Empty Batch Store",
			method:                  http.MethodPost,
			path:                    "/api/shorten/batch",
			body:                    `[]`,
			contentType:             "application/json",
			expectedCode:            http.StatusBadRequest,
			expectedBody:            "{\"error\":\"No items in request\"}\n",
			expectedContentEncoding: "gzip",
		},
		{
			name:                    "Invalid Batch Store",
			method:                  http.MethodPost,
			path:                    "/api/shorten/batch",
			body:                    `[{"original_url": "Https://yandex.Ru"}, {"correlation_id": "uuid2"}]`,
			contentType:             "application/json",
			expectedCode:            http.StatusBadRequest,
			expectedBody:            "{\"error\":\"No valid items in request\"}\n",
			expectedContentEncoding: "gzip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//disable resty redirects
			req := resty.New().SetRedirectPolicy(resty.NoRedirectPolicy()).R().SetDoNotParseResponse(true)
			req.Method = tt.method
			req.SetHeader("Content-Encoding", tt.contentEncoding)
			if tt.contentType != "" {
				req.SetHeader("Content-Type", tt.contentType)
			}
			if tt.contentEncoding == "gzip" {
				var buf bytes.Buffer
				gzipWriter := gzip.NewWriter(&buf)

				_, err := gzipWriter.Write([]byte(tt.body))
				assert.NoError(t, err, "Error comressing request body")

				err = gzipWriter.Close()
				assert.NoError(t, err, "Error closing gzipWriter")
				req.Body = &buf
			} else {
				req.Body = tt.body
			}
			if tt.expectedContentEncoding != "" {
				req.SetHeader("Accept-Encoding", tt.expectedContentEncoding)
			}
			req.URL = srv.URL + tt.path
			log.Println("Performing resty request to URL", req.URL)

			resp, err := req.Send()
			defer resp.RawBody().Close()
			if !errors.Is(err, resty.ErrAutoRedirectDisabled) {
				assert.NoError(t, err, "error making HTTP request")
			}

			assert.Equal(t, tt.expectedCode, resp.StatusCode(), "Response code didn't match expected")

			// проверяем корректность полученного в заголовке редиректа
			if tt.expectedLocation != "" {
				assert.Equal(t, tt.expectedLocation, string(resp.Header().Get("Location")))
			}

			if tt.expectedContentType != "" {
				assert.Equal(t, tt.expectedContentType, string(resp.Header().Get("Content-Type")))
			}

			if tt.expectedContentEncoding != "" {
				assert.Equal(t, tt.expectedContentEncoding, string(resp.Header().Get("Content-Encoding")))
			}

			if tt.expectedContentEncoding == "gzip" && tt.expectedBody != "" {
				gzipReader, err := gzip.NewReader(resp.RawBody())
				assert.NoError(t, err, "error reading gzipped response")
				defer gzipReader.Close()
				unzippedData, err := io.ReadAll(gzipReader)
				assert.NoError(t, err, "error reading unGzipped data")
				log.Println("Decommpressed response data", string(unzippedData))
				assert.Equal(t, tt.expectedBody, string(unzippedData))
			} else if tt.expectedBody != "" {
				log.Println("Resp status", resp.StatusCode(), req.Header, resp.RawResponse, resp.Body())
				assert.Equal(t, tt.expectedBody, string(resp.Body()))
			}
		})
	}
}
