package audit

import "net/http"

type contextKey string

const auditDataKey contextKey = "audit_custom_data"

// Зарезервированные ключи внутри CustomData для хранения системных полей аудита
const (
	actionInternalKey = "_audit_action"
	urlInternalKey    = "_audit_url"
)

// SetField сохраняет произвольное значение по ключу в контекст текущего запроса.
// Используется внутри HTTP-хендлеров для передачи специфичных данных.
func SetField(r *http.Request, key string, value interface{}) {
	if data, ok := r.Context().Value(auditDataKey).(map[string]interface{}); ok {
		data[key] = value
	}
}

// SetAction устанавливает типизированное действие запроса (audit.ActionShorten, audit.ActionFollow).
func SetAction(r *http.Request, action Action) {
	SetField(r, actionInternalKey, action)
}

// SetURL является специализированным хелпером для установки оригинального
// (не сокращенного) URL из хендлера в контекст аудита.
func SetURL(r *http.Request, longURL string) {
	SetField(r, urlInternalKey, longURL)
}
