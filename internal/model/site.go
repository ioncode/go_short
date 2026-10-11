package model

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// short url
type ShortURL string

// long url
type URL string

func (u *URL) UnmarshalJSON(data []byte) error {
	var rawString string
	if err := json.Unmarshal(data, &rawString); err != nil {
		return err
	}
	processed := strings.ToLower(rawString)
	*u = URL(processed)
	return nil
}

// site
type Site struct {
	URL           URL
	ShortURL      ShortURL
	CorrelationID string
	UserID        uuid.UUID
	DeletedFlag   bool
}
