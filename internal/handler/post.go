package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/ioncode/go_short/internal/model"
)

type ShortService interface {
	Short(url model.Url) (model.ShortUrl, error)
	BatchShort(items []model.BatchPostRequestItem) ([]model.BatchPostResponseItem, error)
}

func Post(s ShortService, shortBaseURL string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		log.Println("Started Post handler")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		alias, err := s.Short(model.Url(body))
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		res.WriteHeader(http.StatusCreated)
		url, err := url.JoinPath(shortBaseURL, string(alias))
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		res.Write([]byte(url))
	}
}

func APIPost(s ShortService, shortBaseURL string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		log.Println("Started Api Post handler")
		var requestModel model.PostRequest
		decoder := json.NewDecoder(req.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&requestModel); err != nil {
			writeJSONError(res, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		alias, err := s.Short(requestModel.URL)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusBadRequest)
			return
		}

		url, err := url.JoinPath(shortBaseURL, string(alias))
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusBadRequest)
			return
		}

		result := model.PostResponse{
			Result: url,
		}
		res.WriteHeader(http.StatusCreated)
		json.NewEncoder(res).Encode(result)
	}
}

func APIPostBatch(s ShortService, shortBaseURL string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		log.Println("Started Batch Api Post handler")
		var items []model.BatchPostRequestItem
		decoder := json.NewDecoder(req.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&items); err != nil {
			writeJSONError(res, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		if len(items) < 1 {
			writeJSONError(res, "No items in request", http.StatusBadRequest)
			return
		}

		validItems := items[:0]

		for _, item := range items {
			if item.CorrelationId != "" && item.URL != "" {
				validItems = append(validItems, item)
			}
			log.Println(item)
		}
		if len(validItems) < 1 {
			writeJSONError(res, "No valid items in request", http.StatusBadRequest)
			return
		}
		response, err := s.BatchShort(validItems)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusBadRequest)
			return
		}

		for i, responseItem := range response {
			url, err := url.JoinPath(shortBaseURL, string(responseItem.Alias))
			if err != nil {
				writeJSONError(res, err.Error(), http.StatusBadRequest)
				return
			}
			response[i].Alias = model.ShortUrl(url)
		}
		res.WriteHeader(http.StatusCreated)
		json.NewEncoder(res).Encode(response)
	}
}

// utility function instead http.Error with the same signature
func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(model.ErrorResponse{Error: msg})
}
