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
	ErrSiteExists   = errors.New("Site allready shorted")
	ErrSiteNotFound = errors.New("Site not found")
)

// todo refactor after moving from models
func (sites *Sites) Add(url model.Url) error {
	if sites.Data == nil {
		sites.Data = map[model.ShortUrl]model.Site{}
	}
	//todo move to service
	short := model.ShortUrl("dsdsdsd")

	if _, ok := sites.Data[short]; ok {
		return ErrSiteExists
	}

	site := model.Site{
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
	site, ok := r.sites.Data[alias]
	if ok {
		return site, nil
	} else {
		return model.Site{}, ErrSiteNotFound
	}
}

func (r *MapRepository) StoreSite(site model.Site) error {
	if r.sites.Data == nil {
		r.sites.Data = map[model.ShortUrl]model.Site{}
	}
	if _, ok := r.sites.Data[site.ShortUrl]; ok {
		return ErrSiteExists
	}
	r.sites.Data[site.ShortUrl] = site
	return nil
}
