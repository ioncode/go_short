package service

import "github.com/ioncode/go_short/internal/model"

// repo interface to interact with storage
type SiteRepository interface {
	GetByAlias(alias model.ShortUrl) (model.Site, error)
}

// service interface
type Shortner interface {
	Short(model.Url) (model.ShortUrl, error)
}

// service struct
type MapShortner struct {
	repository SiteRepository
}

//service constructor with DI
func NewShortner(r SiteRepository) *MapShortner {
	return &MapShortner{
		repository: r,
	}
}

func (s *MapShortner) Get(alias model.ShortUrl) (model.Site, error) {
	return s.repository.GetByAlias(alias)
}

func (s MapShortner) Short(Url model.Url) (model.ShortUrl, error) {
	return "short", nil
}
