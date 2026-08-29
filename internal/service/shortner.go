package service

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ioncode/go_short/internal/model"
	"github.com/ioncode/go_short/internal/repository"
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
	Ping(ctx context.Context) error
	Close() error
	BatchStoreSites(sites []model.Site) error
	GetByUser(userId string) ([]model.UserSitesResponseItem, error)
	Delete(ctx context.Context, aliases []model.ShortUrl, user model.User) error
}

type DeleteTask struct {
	Author  model.User
	Aliases []model.ShortUrl
}

// service struct
type Shortner struct {
	repository SiteRepository
	mutex      sync.Mutex
	taskChan   chan DeleteTask // Общий канал-приемник (результат Fan-In)
}

// пул воркеров для асинхронного удаления
const deleteWorkerCount = 10

// буфер канала асинхронного удаления
const deleteBuffer = 10

// service constructor with DI
func NewShortner(r SiteRepository) *Shortner {
	s := &Shortner{
		repository: r,
		taskChan:   make(chan DeleteTask, deleteBuffer),
	}
	for i := range deleteWorkerCount {
		go s.deleteWorker(i)
	}

	return s
}

func (s *Shortner) Enqueue(task DeleteTask) {
	s.taskChan <- task
}

func (s *Shortner) Get(alias model.ShortUrl) (model.Site, error) {
	return s.repository.GetByAlias(alias)
}

func (s *Shortner) Short(url model.Url, user model.User) (model.ShortUrl, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	alias := model.ShortUrl(stringWithCharset(8))
	_, err := s.repository.GetByAlias(alias)
	for err == nil {
		log.Println("This alias allready taken, generating new one", alias)
		alias = model.ShortUrl(stringWithCharset(8))
		_, err = s.repository.GetByAlias(alias)
	}
	site := model.Site{
		Url:      url,
		ShortUrl: alias,
		UserId:   user.ID,
	}
	err = s.repository.StoreSite(site)

	if errors.Is(err, repository.ErrSiteExists) {
		postErr := err
		site, err = s.repository.GetByUrl(url)
		if err == nil {
			err = postErr
		}
	}

	return site.ShortUrl, err
}

func (s *Shortner) BatchShort(items []model.BatchPostRequestItem, user model.User) ([]model.BatchPostResponseItem, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var responseItems []model.BatchPostResponseItem

	var newSites []model.Site

	for _, item := range items {
		site, err := s.repository.GetByUrl(item.URL)

		if err == nil {
			responseItems = append(responseItems, model.BatchPostResponseItem{CorrelationId: item.CorrelationId, Alias: site.ShortUrl})
		} else {
			alias := model.ShortUrl(stringWithCharset(8))
			_, err = s.repository.GetByAlias(alias)
			for err == nil {
				log.Println("This alias allready taken, generating new one", alias)
				alias = model.ShortUrl(stringWithCharset(8))
				_, err = s.repository.GetByAlias(alias)
			}

			responseItems = append(responseItems, model.BatchPostResponseItem{CorrelationId: item.CorrelationId, Alias: alias})
			newSites = append(newSites, model.Site{CorrelationId: item.CorrelationId, Url: item.URL, ShortUrl: alias, UserId: user.ID})
		}
	}

	err := s.repository.BatchStoreSites(newSites)
	return responseItems, err
}

func (s *Shortner) GetByUser(userId string) ([]model.UserSitesResponseItem, error) {
	return s.repository.GetByUser(userId)
}

func (s *Shortner) deleteWorker(workerID int) {
	log.Printf("Worker %d started", workerID)
	for task := range s.taskChan {
		s.processDeleteTask(workerID, task)
	}
}

func (s *Shortner) processDeleteTask(workerID int, task DeleteTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err := s.repository.Delete(ctx, task.Aliases, task.Author)
	if err != nil {
		log.Printf("[Worker %d] Error deleting items for author %s: %v", workerID, task.Author.ID, err)
		return
	}

	log.Printf("[Worker %d] Soft-deleted %d items for author %s", workerID, len(task.Aliases), task.Author.ID)
}
