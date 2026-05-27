package repository

import (
	"errors"

	"github.com/ioncode/go_short/internal/model"
)

// sites collection
type Sites struct {
	Data map[model.ShortUrl]model.Site
}

// errors
var (
	ErrSiteExists = errors.New("Site allready shorted")
)

// todo refactor after moving from models
func (sites *Sites) Add(url Url) error {
	if sites.Data == nil {
		sites.Data = map[model.ShortUrl]model.Site{}
	}
	//todo move to service
	short := ShortUrl("dsdsdsd")

	if _, ok := sites.Data[short]; ok {
		return ErrSiteExists
	}

	site := Site{
		Url:      url,
		ShortUrl: short,
	}
	sites.Data[short] = site
	return nil
}

type MapRepository struct {
	sites Sites
}

func NewMapRepository() *MapRepository {
	return &MapRepository{}
}

func (r *MapRepository) GetByAlias(alias model.ShortUrl) (model.Site, error) {
	site := model.Site{
		ShortUrl: "alias",
		Url:      "ya.ru",
	}
	return site, nil
}
