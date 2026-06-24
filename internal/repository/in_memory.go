package repository

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
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
	file, err := os.OpenFile(storagePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Storage path not opened", err, storagePath)
	}

	repository := MapRepository{
		sites: make(map[model.ShortUrl]model.Site),
		file:  file,
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := bytes.TrimRight(scanner.Bytes(), ", ")
		site := model.Site{}
		err := json.Unmarshal(line, &site)
		if err != nil {
			log.Fatalln("Error reading site from storage", err, string(line))
		}
		repository.sites[site.ShortUrl] = site
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
	writer := bufio.NewWriter(r.file)
	jsonData, err := json.Marshal(site)
	jsonData = append(jsonData, ", \n"...)
	if err != nil {
		return err
	}

	if _, err := writer.Write(jsonData); err != nil {
		return err
	}

	writer.Flush()

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

func (r *MapRepository) Close() error {
	return r.file.Close()
}
