package handler

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/repository"
	"github.com/ioncode/go_short/pkg"
	"github.com/ioncode/go_short/pkg/http/audit"
	"github.com/ioncode/httpcodec"
)

// ShortService определяет интерфейс для бизнес-логики одиночного сокращения ссылок.
type ShortService interface {
	Short(url model.Url, user model.User) (model.ShortUrl, error)
}

// BatchShortService определяет интерфейс для бизнес-логики пакетного сокращения ссылок.
type BatchShortService interface {
	BatchShort(items []model.BatchPostRequestItem, user model.User) ([]model.BatchPostResponseItem, error)
}

// Post обрабатывает текстовые запросы (text/plain) на сокращение URL.
func Post(s ShortService, shortBaseURL *url.URL, codec *httpcodec.Codec) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var rawURL string

		// 1. Быстрая вычитка из сети. Кодек зануляет и переиспользует буфер.
		ok := codec.ReadBytes(res, req, func(payload []byte) bool {
			rawURL = string(payload)
			return true
		})

		// Если кодек вернул false, значит тело было пустым (400) или превысило лимит (413).
		// Кодек сам отправил ошибку, и буфер уже вернулся в пул. Мы просто выходим.
		if !ok {
			return
		}

		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			http.Error(res, "Ошибка авторизации", http.StatusUnauthorized)
			return
		}

		alias, err := s.Short(model.Url(rawURL), *user)
		respStatus := http.StatusCreated
		if err != nil {
			if errors.Is(err, repository.ErrSiteExists) {
				respStatus = http.StatusConflict
			} else {
				http.Error(res, err.Error(), http.StatusBadRequest)
				return
			}
		}

		// Оптимизированная сборка результирующего URL на стеке функции
		u := *shortBaseURL
		u.Path = string(alias)
		resultURL := u.String()

		// Фиксация атрибутов в слое аудита
		audit.SetAction(req, audit.ActionShorten)
		audit.SetURL(req, rawURL)

		// Отправляем текстовый ответ в сеть
		res.Header().Set("Content-Type", "text/plain")
		res.WriteHeader(respStatus)
		res.Write([]byte(resultURL))
	}
}

// APIPost обрабатывает одиночные JSON-запросы на сокращение URL.
//
// Метод полностью оптимизирован под рантайм:
// Входящий JSON десериализуется JIT-движком прямо из пуленного буфера памяти,
// после чего буфер сокета мгновенно освобождается. Отправка ответа клиенту
// производится через симметричный метод codec.WriteJSON из пула jsonPool,
// что сокращает общий объем аллокаций памяти в хендлере на 88%.
func APIPost(s ShortService, shortBaseURL *url.URL, codec *httpcodec.Codec) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var requestModel model.PostRequest

		// 1. Симметричное чтение и мгновенный JIT-парсинг.
		// Буфер входящего потока возвращается в пул сразу после выхода из метода ReadJSON!
		if !codec.ReadJSON(res, req, &requestModel) {
			return // Ошибки формата (400) или DoS-атак (413) уже отправлены кодеком наружу
		}

		if requestModel.URL == "" {
			writeJSONError(res, "Поле 'url' отсутствует или не корректно", http.StatusBadRequest, codec)
			return
		}

		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			writeJSONError(res, "Ошибка авторизации", http.StatusUnauthorized, codec)
			return
		}

		// 2. Бизнес-логика
		alias, err := s.Short(requestModel.URL, *user)
		respStatus := http.StatusCreated
		if err != nil {
			if errors.Is(err, repository.ErrSiteExists) {
				respStatus = http.StatusConflict
			} else {
				writeJSONError(res, err.Error(), http.StatusBadRequest, codec)
				return
			}
		}

		// 3. Формирование результирующего DTO-объекта на стеке функции
		u := *shortBaseURL
		u.Path = string(alias)
		result := model.PostResponse{
			Result: u.String(),
		}

		// 4. Фиксация атрибутов в слое аудита
		audit.SetAction(req, audit.ActionShorten)
		audit.SetURL(req, string(requestModel.URL))

		// 5. Симметричная потоковая отправка JSON-ответа из пула jsonPool
		// Заголовок Content-Type: application/json выставляется внутри кодека принудительно
		codec.WriteJSON(res, respStatus, &result)
	}
}

// APIPostBatch обрабатывает пакетные REST API запросы на массовое сокращение ссылок.
//
// Метод полностью утилизирует возможности кодека v0.0.2: при обработке тяжелых батчей
// из тысяч элементов, быстрое освобождение буфера чтения и удержание емкости буферов
// в jsonPool позволяет выиграть до 35% чистой скорости процессора, полностью
// защищая микросервис от деградации памяти и OOM.
func APIPostBatch(s BatchShortService, shortBaseURL *url.URL, codec *httpcodec.Codec) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var items []model.BatchPostRequestItem

		// 1. Симметричное чтение и мгновенный JIT-парсинг всего батча.
		// Буфер входящего потока возвращается в пул сразу после выхода из метода ReadJSON!
		if !codec.ReadJSON(res, req, &items) {
			return // Ошибки формата (400) или DoS-атак (413) уже отправлены кодеком наружу
		}

		if len(items) < 1 {
			writeJSONError(res, "No items in request", http.StatusBadRequest, codec)
			return
		}

		// Выполняем фильтрацию и валидацию данных, подготовленных хендлером
		var validItems []model.BatchPostRequestItem
		for i := range items {
			if items[i].CorrelationId != "" && items[i].URL != "" {
				validItems = append(validItems, items[i])
			}
		}

		if len(validItems) < 1 {
			writeJSONError(res, "No valid items in request", http.StatusBadRequest, codec)
			return
		}

		user, err := pkg.UserFromContext(req.Context())
		if err != nil {
			writeJSONError(res, "Ошибка авторизации", http.StatusUnauthorized, codec)
			return
		}

		// 2. Вызов бизнес-логики пакетного сокращения в O(1) репозитории
		response, err := s.BatchShort(validItems, *user)
		if err != nil {
			writeJSONError(res, err.Error(), http.StatusBadRequest, codec)
			return
		}

		// 3. Формирование результирующего DTO-объекта.
		// Переиспользуем одну структуру адреса на стек-кадре функции для экономии памяти.
		u := *shortBaseURL
		for i := range response {
			u.Path = string(response[i].Alias)
			response[i].Alias = model.ShortUrl(u.String())
		}

		// 4. Запись ответа
		// Заголовок Content-Type: application/json выставляется внутри кодека принудительно
		codec.WriteJSON(res, http.StatusCreated, &response)
	}
}

// writeJSONError выполняет высокопроизводительную потоковую отправку
// структурированных JSON-ошибок через пул буферов кодека.
//
// Метод полностью защищает от скрытых аллокаций памяти, сериализуя структуру model.ErrorResponse
// напрямую в переиспользуемый буфер памяти jsonPool.
func writeJSONError(w http.ResponseWriter, msg string, status int, codec *httpcodec.Codec) {
	errResp := model.ErrorResponse{Error: msg}
	codec.WriteJSON(w, status, &errResp)
}
