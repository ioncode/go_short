package handler_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ioncode/go_short/internal/config"
	"github.com/ioncode/go_short/internal/handler"
	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/pkg"
)

type MockService struct {
	MockShort func(url string) (string, error)
}

func (m *MockService) Short(url model.Url, user model.User) (model.ShortUrl, error) {
	alias, error := m.MockShort(string(url))
	return model.ShortUrl(alias), error
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
			handler := handler.Post(service, "http://localhost:8080/")
			ctx := pkg.WithUser(context.Background(), &model.User{ID: "f2a2f7ef-bfd5-44be-ba21-fc91af79733e"})
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/", strings.NewReader("ya.ru"))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			res := w.Result()
			defer res.Body.Close()
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
