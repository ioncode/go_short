package repository

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"os"
	"slices"
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
	file  *os.File
}

func NewMapRepository(storagePath string) *MapRepository {
	file, err := os.OpenFile(storagePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalln("Storage path not opened", err, storagePath)
	}

	repository := MapRepository{
		sites: make(map[model.ShortUrl]model.Site),
		file:  file,
	}

	reader := bufio.NewReader(file)
	decoder := json.NewDecoder(reader)
	t, err := decoder.Token()
	if err != nil && err != io.EOF {
		log.Fatalf("Failed to read token: %v", err)
	}
	if t != nil {
		for decoder.More() {
			var site model.Site
			err := decoder.Decode(&site)
			if err != nil {
				log.Fatalf("Failed to decode site : %v", err)
			}
			repository.sites[site.ShortUrl] = site
		}
	}

	return &repository
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
	if _, ok := r.sites[site.ShortUrl]; ok {
		return ErrSiteExists
	}

	for _, storedSite := range r.sites {
		if site.Url == storedSite.Url {
			return ErrSiteExists
		}
	}

	return r.flushToFile()
}

func (r *MapRepository) BatchStoreSites(sites []model.Site) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, site := range sites {
		r.sites[site.ShortUrl] = site
	}

	return r.flushToFile()
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

func (r *MapRepository) Close() error {
	return r.file.Close()
}

func (r *MapRepository) Ping(context.Context) error {
	return errors.New("Ping not supported for Map repository")
}

func (r *MapRepository) GetByUser(userId string) ([]model.UserSitesResponseItem, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	result := []model.UserSitesResponseItem{}
	for _, site := range r.sites {
		if site.UserId == userId {
			result = append(result, model.UserSitesResponseItem{
				Alias: site.ShortUrl,
				URL:   site.Url,
			})
		}
	}
	return result, nil
}

func (r *MapRepository) Delete(ctx context.Context, aliases []model.ShortUrl, user model.User) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	var changed bool
	for _, alias := range aliases {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, exists := r.sites[alias]
		if exists && item.UserId == user.ID && !item.DeletedFlag {
			item.DeletedFlag = true
			r.sites[alias] = item
			changed = true
		}
	}
	// Записываем в файл только если были реальные изменения флагов
	if changed {
		return r.flushToFile()
	}

	return nil
}

// flushToFile перезаписывает файл текущим состоянием r.sites.
// Должен вызываться под заблокированным r.mutex.
func (r *MapRepository) flushToFile() error {
	info, err := r.file.Stat()
	if err != nil {
		return err
	}

	if info.Size() > 0 {
		err := r.file.Truncate(0)
		if err != nil {
			return err
		}
		_, err = r.file.Seek(0, 0)
		if err != nil {
			return err
		}
	}

	writer := bufio.NewWriter(r.file)
	allSites := slices.Collect(maps.Values(r.sites))
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(allSites); err != nil {
		return err
	}
	return writer.Flush()
}
