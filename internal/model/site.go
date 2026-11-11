package model

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// ShortURL представляет собой уникальный текстовый идентификатор (алиас) короткой ссылки.
type ShortURL string

// URL представляет собой полный оригинальный веб-адрес, прошедший валидацию и нормализацию.
type URL string

// UnmarshalJSON реализует интерфейс json.Unmarshaler для типа URL.
// Метод автоматически приводит входящую строку с адресом к нижнему регистру для консистентного хранения.
func (u *URL) UnmarshalJSON(data []byte) error {
	var rawString string
	if err := json.Unmarshal(data, &rawString); err != nil {
		return err
	}
	processed := strings.ToLower(rawString)
	*u = URL(processed)
	return nil
}

// Site описывает внутреннюю сущность сокращенной ссылки, хранящуюся в базе данных сервиса.
type Site struct {
	// URL содержит оригинальный длинный адрес веб-сайта.
	URL URL
	// ShortURL содержит сгенерированный короткий алиас для этого адреса.
	ShortURL ShortURL
	// CorrelationID используется для связи элемента с клиентским запросом при пакетной обработке.
	CorrelationID string
	// UserID хранит уникальный идентификатор пользователя, создавшего эту ссылку.
	UserID uuid.UUID
	// DeletedFlag указывает, была ли ссылка помечена пользователем как удаленная (soft delete).
	DeletedFlag bool
}
