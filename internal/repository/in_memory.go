package repository

import (
	"errors"
	"sync"

	"github.com/ioncode/go_short/internal/model"
)

// errors
var (
	ErrSiteExists   = errors.New("Site allready shorted")
	ErrSiteNotFound = errors.New("Site not found")
)

type MapRepository struct {
	sites map[model.ShortUrl]model.Site
	mutex sync.Mutex
}

func NewMapRepository() *MapRepository {
	return &MapRepository{}
}

func (r *MapRepository) GetByAlias(alias model.ShortUrl) (model.Site, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	site, ok := r.sites[alias]
	if ok {
		return site, nil
	} else {
		return model.Site{}, ErrSiteNotFound
	}
}

func (r *MapRepository) StoreSite(site model.Site) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.sites == nil {
		r.sites = map[model.ShortUrl]model.Site{}
	}
	if _, ok := r.sites[site.ShortUrl]; ok {
		return ErrSiteExists
	}
	r.sites[site.ShortUrl] = site
	return nil
}

func (r *MapRepository) GetByUrl(url model.Url) (model.Site, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, site := range r.sites {
		if site.Url == url {
			return site, nil
		}
	}

	return model.Site{}, ErrSiteNotFound

}
