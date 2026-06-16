package handler_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ioncode/go_short/internal/handler"
	"github.com/ioncode/go_short/internal/model"
)

type MockGetService struct {
	MockGet func(alias string) (string, error)
}

func (m *MockGetService) Get(alias model.ShortUrl) (model.Site, error) {
	Url, error := m.MockGet(string(alias))
	site := model.Site{
		Url:      model.Url(Url),
		ShortUrl: alias,
	}
	return site, error
}

func TestGet(t *testing.T) {
	tests := []struct {
		name           string
		expectedStatus int
		expectedBody   string
		mockBehavior   func(url string) (string, error)
	}{
		{
			name:           "Correct request",
			expectedStatus: http.StatusTemporaryRedirect,
			expectedBody:   "<a href=\"/ya.ru\">Temporary Redirect</a>.\n\n",
			mockBehavior: func(url string) (string, error) {
				return "ya.ru", nil
			},
		},
		{
			name:           "Service error",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   ": Some problems in service\n",
			mockBehavior: func(url string) (string, error) {
				return "", errors.New("Some problems in service")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &MockGetService{
				MockGet: tt.mockBehavior,
			}
			handler := handler.Get(service)
			req := httptest.NewRequest(http.MethodGet, "/123", nil)
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
