package service

import (
	"log"
	"math/rand/v2"

	"github.com/ioncode/go_short/internal/model"
)

// fast random string generator
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func stringWithCharset(length int) string {
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
	GetByUrl(url model.Url) (model.Site, error)
}

// service struct
type Shortner struct {
	repository SiteRepository
}

// service constructor with DI
func NewShortner(r SiteRepository) *Shortner {
	return &Shortner{
		repository: r,
	}
}

func (s *Shortner) Get(alias model.ShortUrl) (model.Site, error) {
	return s.repository.GetByAlias(alias)
}

func (s *Shortner) Short(url model.Url) (model.ShortUrl, error) {

	site, err := s.repository.GetByUrl(url)

	if err == nil {
		return site.ShortUrl, nil
	}

	alias := model.ShortUrl(stringWithCharset(8))
	for _, err := s.repository.GetByAlias(alias); err == nil; alias = model.ShortUrl(stringWithCharset(8)) {
		log.Println("This alias allready taken, generating new one", alias)
	}
	site = model.Site{
		Url:      url,
		ShortUrl: alias,
	}
	err = s.repository.StoreSite(site)

	return site.ShortUrl, err
}
