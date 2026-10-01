package router

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ioncode/httpcodec"
)

func Test_middleware(t *testing.T) {
	codec := httpcodec.New(7000)
	responseContentTypeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%#v\n", r.Header)
		log.Printf("%#v\n", w)
		cors := w.Header().Get("Access-Control-Allow-Origin")
		if cors != "*" {
			t.Error("CORS headers not correct:", cors)
		}
		_, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(w, "Too Large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		next           http.Handler
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
				cors := w.Header().Get("Access-Control-Allow-Origin")
				if cors != "*" {
					t.Error("CORS headers not correct:", cors)
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
		{
			name:           "Post with correct content type and too large body",
			method:         http.MethodPost,
			expectedStatus: http.StatusRequestEntityTooLarge,
			body:           strings.NewReader(strings.Repeat("S", 10000)),
			contentType:    "text/plain; charset=utf-8",
			next:           responseContentTypeHandler,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", tt.body)
			req.Header.Set("Content-type", tt.contentType)
			rec := httptest.NewRecorder()
			mw := codec.Middleware()
			middleware := mw(corsMiddleware(tt.next))
			middleware.ServeHTTP(rec, req)
			res := rec.Result()
			defer res.Body.Close()
			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d; got %d", tt.expectedStatus, res.StatusCode)
			}
		})
	}
}
