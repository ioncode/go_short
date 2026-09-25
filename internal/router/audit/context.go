package audit

import "net/http"

type contextKey string

const auditDataKey contextKey = "audit_custom_data"

// Зарезервированные ключи внутри CustomData для хранения системных полей аудита
const (
	actionInternalKey = "_audit_action"
	urlInternalKey    = "_audit_url"
)

// SetField сохраняет произвольное значение по ключу в контейнер контекста
func SetField(r *http.Request, key string, value any) {
	if container, ok := r.Context().Value(auditDataKey).(*auditContainer); ok {
		container.mu.Lock()
		container.data[key] = value
		container.mu.Unlock()
	}
}

// SetAction устанавливает типизированное действие запроса (audit.ActionShorten, audit.ActionFollow).
func SetAction(r *http.Request, action Action) {
	SetField(r, actionInternalKey, action)
}

// SetURL устанавливает оригинальный URL из хендлера
func SetURL(r *http.Request, longURL string) {
	SetField(r, urlInternalKey, longURL)
}
