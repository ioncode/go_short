package model

import (
	"encoding/json"
	"strings"
)

// short url
type ShortUrl string

// long url
type Url string

func (u *Url) UnmarshalJSON(data []byte) error {
	var rawString string
	if err := json.Unmarshal(data, &rawString); err != nil {
		return err
	}
	processed := strings.ToLower(rawString)
	*u = Url(processed)
	return nil
}

// site
type Site struct {
	Url      Url
	ShortUrl ShortUrl
}
