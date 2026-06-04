package router

import (
	"log"
	"mime"
	"net/http"

	"github.com/ioncode/go_short/internal/handler"
	"github.com/ioncode/go_short/internal/repository"
	"github.com/ioncode/go_short/internal/service"
)

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "text/plain")
		log.Println("Middleware processing request with length ", r.ContentLength)
		if r.ContentLength > 700 {
			http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)
			return
		} else if r.ContentLength > 0 {
			contentType := r.Header.Get("Content-type")
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err != nil {
				http.Error(w, "Invalid Content-Type header", http.StatusBadRequest)
				log.Println("Error parsing content type:", contentType)
				return
			}
			if mediaType != "text/plain" {
				http.Error(w, "Content type not correct", http.StatusBadRequest)
				log.Println("Invalid content type for request: ", contentType, mediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type Server struct {
	ShortnerService service.Shortner
}

func Serve() {
	mux := http.NewServeMux()

	repo := repository.NewMapRepository()

	service := service.NewShortner(repo)

	mux.HandleFunc(`GET /{alias}`, handler.Get(service))
	mux.HandleFunc(`POST /`, handler.Post(service))

	log.Fatal(http.ListenAndServe(":8080", middleware(mux)))
}
