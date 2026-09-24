// Package audit предоставляет потокобезопасную асинхронную систему
// логирования и аудита HTTP-запросов, основанную на паттерне Наблюдатель (Observer)
// с использованием пула воркеров и каналов.
package audit

import (
	"context"
)

// Action определяет тип действия, совершенного пользователем.
type Action string

const (
	// ActionShorten указывает на действие создания (сокращения) ссылки.
	ActionShorten Action = "shorten"

	// ActionFollow указывает на прохождение (редирект) по сокращенной ссылке.
	ActionFollow Action = "follow"
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
	UserID string `json:"user_id,omitempty"`

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
