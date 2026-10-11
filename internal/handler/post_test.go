package handler_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/handler"
	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/pkg/http/request/payload"
	"github.com/ioncode/httpcodec"
)

type MockService struct {
	MockShort func(url string) (string, error)
}

func (m *MockService) Short(url model.URL, user model.User) (model.ShortURL, error) {
	alias, error := m.MockShort(string(url))
	return model.ShortURL(alias), error
}
func TestPost(t *testing.T) {

	config.ParseFlags()
	tests := []struct {
		name           string
		expectedStatus int
		expectedBody   string
		mockBehavior   func(url string) (string, error)
	}{
		{
			name:           "Correct request",
			expectedStatus: http.StatusCreated,
			expectedBody:   "http://localhost:8080/123",
			mockBehavior: func(url string) (string, error) {
				return "123", nil
			},
		},
		{
			name:           "Service error",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Some problems in service\n",
			mockBehavior: func(url string) (string, error) {
				return "", errors.New("Some problems in service")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &MockService{
				MockShort: tt.mockBehavior,
			}
			bu, _ := url.Parse("http://localhost:8080/")
			codec := httpcodec.New(
				8192,
				httpcodec.WithMaxTrashRead(64*1024), // Очистка до 64 КБ для сохранения Keep-Alive сессий
				httpcodec.WithInitJSONBufferCap(4*1024),  // Старт буфера ответа с 4 КБ
				httpcodec.WithMaxJSONBufferCap(256*1024), // Защита от OOM: жесткий лимит буфера ответа 256 КБ
			)
			handler := handler.Post(service, bu, codec)

			// 1. Создаем контекст с payload и получаем функцию очистки
			ctx, teardown := newTestContextWithPayload(uuid.MustParse("f2a2f7ef-bfd5-44be-ba21-fc91af79733e"))

			// 2. Гарантируем возврат структуры в пул сразу после завершения текущего тест-кейса
			defer teardown()
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/", strings.NewReader("ya.ru"))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			res := w.Result()
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d; got %d", tt.expectedStatus, res.StatusCode)
			}
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			if string(body) != tt.expectedBody {
				t.Errorf("expected body %q; got %q", tt.expectedBody, string(body))
			}

		})
	}
}

// newTestContextWithPayload инициализирует чистый Payload из пула, наполняет его
// переданным userID и возвращает контекст вместе с функцией очистки (teardown).
func newTestContextWithPayload(userID uuid.UUID) (context.Context, func()) {
	// 1. Берем ОДИН контейнер из реального пула пакета payload
	p := payload.Get()

	// 2. Инициализируем базовый контекст с этим контейнером
	ctx := context.WithValue(context.Background(), payload.PayloadContextKey, p)

	// 3. Создаем временный запрос, чтобы безопасно вызвать экспортируемый API: payload.SetAuthorID
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", nil)
	payload.SetAuthorID(req, userID)

	// 4. Возвращаем готовый контекст запроса и функцию возврата структуры в пул
	return req.Context(), func() {
		p.Release()
	}
}
