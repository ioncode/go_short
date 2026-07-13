package handler

import (
	"context"
	"net/http"
	"time"
)

type PingService interface {
	Ping(ctx context.Context) error
}

func Ping(s PingService) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := s.Ping(ctx)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
