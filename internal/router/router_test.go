package router

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_middleware(t *testing.T) {

	responseContentTypeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%#v\n", r.Header)
		log.Printf("%#v\n", w)
		contentType := w.Header().Get("Content-type")
		if contentType != "text/plain" {
			t.Error("Content type not correct:", contentType)
		}
	})

	tests := []struct {
		name           string
		next           http.Handler // we can keep dynamic parameter, but now use only static responseContentTypeHandler for current middleware implementation
		method         string
		expectedStatus int
		body           io.Reader
		contentType    string
	}{
		{
			name: "Get request without payload",
			next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				log.Printf("%#v\n", r.Header)
				log.Printf("%#v\n", w)
				contentType := w.Header().Get("Content-type")
				if contentType != "text/plain" {
					t.Error("Content type not correct:", contentType)
				}
			}),
			expectedStatus: http.StatusOK,
			method:         http.MethodGet,
			body:           nil,
		},
		{
			name:           "Post with correct content type and body",
			method:         http.MethodPost,
			expectedStatus: http.StatusOK,
			body:           strings.NewReader("ya.ru"),
			contentType:    "text/plain; charset=utf-8",
			next:           responseContentTypeHandler,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", tt.body)
			req.Header.Set("Content-type", tt.contentType)
			rec := httptest.NewRecorder()
			middleware(tt.next).ServeHTTP(rec, req)
			res := rec.Result()
			defer res.Body.Close()
			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d; got %d", tt.expectedStatus, res.StatusCode)
			}
		})
	}
}
