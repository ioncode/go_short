package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/repository"
	"github.com/ioncode/go_short/pkg"
)

type ShortService interface {
	Short(url model.Url, user model.User) (model.ShortUrl, error)
}

type BatchShortService interface {
	BatchShort(items []model.BatchPostRequestItem, user model.User) ([]model.BatchPostResponseItem, error)
}

// requestPool переиспользует оперативную память для чтения входящих HTTP-запросов.
// Устанавливаем емкость 8192 байта (8 КБ), что с запасом перекрывает
// системный лимит в 7000 байт, заданный в requestContentLengthMiddleware.
var requestPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 8192))
	},
}

// Post обрабатывает текстовые запросы на сокращение URL.
func Post(s ShortService, shortBaseURL string) http.HandlerFunc {
	// Предварительно очищаем базовый URL от слэшей справа ОДИН раз при инициализации роутера.
	// Это избавляет приложение от вызова тяжелого url.JoinPath на каждый входящий запрос.
	trimmedBaseURL := strings.TrimSuffix(shortBaseURL, "/")

	return func(res http.ResponseWriter, req *http.Request) {
		// 1. Извлекаем буфер из пула памяти
		buf := requestPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer requestPool.Put(buf)

		// 2. Вычитываем тело запроса напрямую в пуленный буфер.
		// Ограничение в 7000 байт уже контролируется http.MaxBytesReader из middleware.
		_, err := buf.ReadFrom(req.Body)
		if err != nil {
			// Если middleware оборвал чтение из-за превышения лимита, возвращаем Bad Request
			http.Error(res, "Превышен максимальный размер тела запроса", http.StatusBadRequest)
			return
		}

		bodyBytes := buf.Bytes()
		if len(bodyBytes) == 0 {
			http.Error(res, "Тело запроса не может быть пустым", http.StatusBadRequest)
			return
		}

		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}

		// 3. Бизнес-логика создания сокращенной ссылки
		alias, err := s.Short(model.Url(bodyBytes), *user)
		respStatus := http.StatusCreated
		if err != nil {
			if errors.Is(err, repository.ErrSiteExists) {
				respStatus = http.StatusConflict
			} else {
				http.Error(res, err.Error(), http.StatusBadRequest)
				return
			}
		}

		res.WriteHeader(respStatus)

		// 4. Оптимизированная сборка результирующей строки.
		// Конкатенация строк в Go 1.26+ эффективно выделяет память за один проход аллокатора.
		resultURL := trimmedBaseURL + "/" + string(alias)

		res.Write([]byte(resultURL))
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
		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}
		alias, err := s.Short(requestModel.URL, *user)
		respStatus := http.StatusCreated
		if err != nil {
			if errors.Is(err, repository.ErrSiteExists) {
				respStatus = http.StatusConflict
			} else {
				writeJSONError(res, err.Error(), http.StatusBadRequest)
				return
			}
		}

		url, err := url.JoinPath(shortBaseURL, string(alias))
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusBadRequest)
			return
		}

		result := model.PostResponse{
			Result: url,
		}
		res.WriteHeader(respStatus)
		json.NewEncoder(res).Encode(result)
	}
}

func APIPostBatch(s BatchShortService, shortBaseURL string) http.HandlerFunc {
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

		var validItems []model.BatchPostRequestItem

		for _, item := range items {
			if item.CorrelationId != "" && item.URL != "" {
				validItems = append(validItems, item)
			}
		}
		if len(validItems) < 1 {
			writeJSONError(res, "No valid items in request", http.StatusBadRequest)
			return
		}
		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}
		response, err := s.BatchShort(validItems, *user)
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
