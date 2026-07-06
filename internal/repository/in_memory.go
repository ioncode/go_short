package repository

import (
	"bufio"
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
	r.sites[site.ShortUrl] = site
	//create slice of sites for storage
	sites := slices.Collect(maps.Values(r.sites))
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sites); err != nil {
		return err
	}
	writer.Flush()

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

func (r *MapRepository) Close() error {
	return r.file.Close()
}
