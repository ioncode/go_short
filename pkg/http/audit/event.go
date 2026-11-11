// Package audit предоставляет потокобезопасную асинхронную систему
// логирования и аудита HTTP-запросов, основанную на паттерне Наблюдатель (Observer)
// с использованием пула воркеров и каналов.
package audit

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ioncode/go_short/pkg/http/request/payload"
)

// Action определяет тип действия, совершенного пользователем.
type Action string

const (
	// ActionShorten указывает на действие создания (сокращения) ссылки.
	ActionShorten Action = "shorten"

	// ActionFollow указывает на прохождение (редирект) по сокращенной ссылке.
	ActionFollow Action = "follow"

	// внутренние ключи пакета для payload
	actionPayloadKey     = "audit_action"
	urlPayloadKey        = "audit_url"
	customDataPayloadKey = "audit_custom_data"
)

// Event описывает структуру события успешного HTTP-запроса,
// которое передается всем зарегистрированным наблюдателям.
type Event struct {
	// TS содержит время события в формате Unix timestamp (секунды).
	TS int64 `json:"ts"`

	// Method содержит HTTP-метод запроса (GET, POST, PUT и т.д.).
	Method string `json:"-"`

	// Path содержит запрошенный URL-путь.
	Path string `json:"-"`

	// StatusCode содержит HTTP-статус ответа (гарантированно в диапазоне 200-399).
	StatusCode int `json:"-"`

	// UserID содержит идентификатор пользователя, извлеченный из контекста запроса.
	// Если пользователь анонимен или не был определен, поле не передается при парсинге в JSON.
	UserID *uuid.UUID `json:"user_id,omitempty"`

	// Action указывает тип совершенного действия.
	Action Action `json:"action"`

	// URL содержит оригинальный (длинный) URL, с которым производилось действие.
	// Заполняется хендлером при создании ссылки или при редиректе.
	URL string `json:"url"`

	// CustomData содержит произвольные дополнительные данные, добавленные из хендлера.
	CustomData map[string]any `json:"-"`
}

// Observer определяет интерфейс для подписчиков (приемников) системы аудита.
type Observer interface {
	// Name возвращает человекочитаемое имя приемника (например, "Файловый аудит").
	Name() string

	// OnRequest вызывается воркером асинхронно для каждого события.
	OnRequest(ctx context.Context, event Event)
}

// NewEvent выполняет роль фабрики-конструктора (Snapshot) для структуры Event.
// Она атомарно извлекает доменные метаданные из переиспользуемой шины payload.Payload
// текущего запроса и формирует изолированный объект события для передачи в асинхронную очередь.
//
// Если в рамках текущего запроса хендлерами не было зафиксировано доменное действие (Action),
// конструктор возвращает пустую структуру и флаг false, сигнализируя мидлвари о необходимости
// пропустить этап логирования холостого запроса.
//
// Для предотвращения Data Race в конкурентной среде, метод выполняет глубокое физическое
// копирование динамической мапы CustomData с помощью функции maps.Copy.
func NewEvent(r *http.Request, statusCode int) (Event, bool) {
	ctx := r.Context()

	// 1. Извлекаем совершенное действие (Action) из пуленой шины памяти
	var action Action
	if val, exists := payload.GetCustomValue(ctx, actionPayloadKey); exists {
		if act, ok := val.(Action); ok {
			action = act
		}
	}

	// Если действие отсутствует, значит этот запрос не подлежит аудиту (например, /ping или статика)
	if action == "" {
		return Event{}, false
	}

	// 2. Извлекаем целевой URL (оригинальный или сокращенный)
	var targetURL string
	if val, exists := payload.GetCustomValue(ctx, urlPayloadKey); exists {
		if u, ok := val.(string); ok {
			targetURL = u
		}
	}

	// 3. БЕЗОПАСНЫЙ СНАПШОТ ДЛЯ CUSTOM DATA (Глубокое копирование)
	var customDataCopy map[string]any
	if val, exists := payload.GetCustomValue(ctx, customDataPayloadKey); exists {
		if m, ok := val.(map[string]any); ok && len(m) > 0 {
			// Выделяем память под мапу строго под размер исходных данных
			customDataCopy = make(map[string]any, len(m))
			maps.Copy(customDataCopy, m)
		}
	}

	// 4. Считываем атомарно скомпилированный бинарный ID пользователя
	authorID := payload.GetAuthorID(ctx)

	var userIDPtr *uuid.UUID
	if authorID != uuid.Nil {
		userIDCopy := authorID // Делаем локальную копию для безопасности
		userIDPtr = &userIDCopy
	}
	// 5. Конструируем событие на стеке текущей горутины.
	// Объект передается в канал по значению (копированием байт), что исключает аллокации в куче.
	event := Event{
		TS:         time.Now().Unix(),
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: statusCode,
		UserID:     userIDPtr,
		Action:     action,
		URL:        targetURL,
		CustomData: customDataCopy,
	}

	return event, true
}
