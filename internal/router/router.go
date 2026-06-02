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
		w.Header().Set("Content-Type", "text/plain")
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
