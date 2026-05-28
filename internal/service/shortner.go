package service

import (
	"math/rand/v2"

	"github.com/ioncode/go_short/internal/model"
)

// fast random string generator
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func StringWithCharset(length int) string {
	b := make([]byte, length)
	for i := range b {
		// Use IntN to pick a random index from the charset
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

// repo interface to interact with storage
type SiteRepository interface {
	GetByAlias(alias model.ShortUrl) (model.Site, error)
	StoreSite(site model.Site) error
}

// service interface
type Shortner interface {
	Short(model.Url) (model.ShortUrl, error)
}

// service struct
type MapShortner struct {
	repository SiteRepository
}

// service constructor with DI
func NewShortner(r SiteRepository) *MapShortner {
	return &MapShortner{
		repository: r,
	}
}

func (s *MapShortner) Get(alias model.ShortUrl) (model.Site, error) {
	return s.repository.GetByAlias(alias)
}

func (s *MapShortner) Short(Url model.Url) (model.ShortUrl, error) {

	//todo check if site allready stored

	alias := model.ShortUrl(StringWithCharset(8))
	site := model.Site{
		Url:      Url,
		ShortUrl: alias,
	}
	error := s.repository.StoreSite(site)

	return site.ShortUrl, error
}
