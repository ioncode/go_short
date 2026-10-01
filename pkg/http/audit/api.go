package audit

import (
	"net/http"

	"github.com/ioncode/go_short/pkg/http/request/payload"
)

// SetAction фиксирует тип совершаемого доменного действия внутри шины полезной нагрузки запроса.
// Метод вызывается внутри HTTP-хендлеров при успешном начале выполнения операции.
//
// Пример использования:
//
//	audit.SetAction(req, audit.ActionShorten)
func SetAction(r *http.Request, action Action) {
	payload.SetCustomValue(r, actionPayloadKey, action)
}

// SetURL фиксирует оригинальный длинный URL
// внутри шины полезной нагрузки текущего запроса.
//
// Пример использования:
//
//	audit.SetURL(req, "https://yandex.ru")
func SetURL(r *http.Request, targetURL string) {
	payload.SetCustomValue(r, urlPayloadKey, targetURL)
}

// SetCustomData фиксирует произвольные дополнительные данные (бизнес-метрики)
// внутри шины полезной нагрузки текущего запроса.
// При передаче данных в асинхронный логгер метод выполнит глубокое копирование мапы.
//
// Пример использования:
//
//	audit.SetCustomData(req, map[string]any{"batch_size": len(items)})
func SetCustomData(r *http.Request, data map[string]any) {
	payload.SetCustomValue(r, customDataPayloadKey, data)
}
