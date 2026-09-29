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

const (
	// charset содержит набор алфавитно-цифровых символов, используемых
	// для генерации коротких буквенно-цифровых алиасов.
	charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// maxShortenRetries определяет максимальное количество попыток
	// генерации уникального алиаса при коллизиях в конкурентной среде.
	maxShortenRetries = 3
)

// stringWithCharset генерирует псевдослучайную строку заданной длины,
// используя символы из константы charset.
//
// Функция полностью потокобезопасна (использует lock-free генератор math/rand/v2)
// и оптимизирована для работы на стеке текущей горутины без лишних аллокаций в куче.
func stringWithCharset(length int) string {
	b := make([]byte, length)
	for i := range b {
		// math/rand/v2.IntN работает быстрее старого пакета math/rand
		// и не требует глобальной блокировки сида (seed).
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

// repo interface to interact with storage
type SiteRepository interface {
	GetByAlias(alias model.ShortUrl) (model.Site, error)
	StoreSite(site model.Site) (model.ShortUrl, error)
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

// Short выполняет конкурентное высокопроизводительное сокращение оригинального URL.
//
// Метод полностью избавлен от блокировок (s.mutex удален) и реализует паттерн
// оптимистичной записи. Он сразу пытается сохранить сгенерированный алиас в репозиторий,
// делегируя проверку уникальности на уровень хранилища под его внутреннюю атомарную блокировку.
//
// В случае коллизии сгенерированного алиаса метод выполняет повторную попытку (до 3 раз).
// Если оригинальный URL уже существует в базе (был создан ранее или в параллельной гонке запросов),
// метод атомарно возвращает существующий сохраненный алиас и ошибку repository.ErrSiteExists.
//
// Возвращаемые значения:
//   - model.ShortUrl: сгенерированный или уже существующий короткий алиас (например, "aB34ef7X").
//   - error: nil при успехе; repository.ErrSiteExists при дубликате оригинального URL;
//     системная ошибка, если лимит попыток исчерпан или недоступно хранилище.
func (s *Shortner) Short(url model.Url, user model.User) (model.ShortUrl, error) {
	// Запускаем цикл оптимистичной вставки с лимитом итераций (Go 1.22+ синтаксис).
	// Fail-Fast: защищает горутину от зависания и DoS-эффекта при системных сбоях.
	for range maxShortenRetries {
		// Генерируем случайный текстовый идентификатор на стеке горутины
		alias := model.ShortUrl(stringWithCharset(8))

		site := model.Site{
			Url:      url,
			ShortUrl: alias,
			UserId:   user.ID,
		}

		// Выполняем строго один атомарный запрос к репозиторию.
		// Новая сигнатура StoreSite возвращает (model.ShortUrl, error) из-под своего Lock.
		existingAlias, err := s.repository.StoreSite(site)
		if err == nil {
			return alias, nil // Новая ссылка успешно создана и сохранена
		}

		// Сценарий А: Оригинальный URL уже существует в базе (параллельный запрос выиграл гонку)
		if errors.Is(err, repository.ErrSiteExists) {
			// Возвращаем ранее созданный алиас и ошибку-маркер для хэндлера (HTTP 409 Conflict)
			return existingAlias, err
		}

		// Сценарий Б: Сгенерированная строка совпала с чужим алиасом в базе (редчайшая коллизия)
		if errors.Is(err, repository.ErrAliasConflict) {
			// Пропускаем шаг и переходим к следующей итерации для генерации новой строки
			continue
		}

		// Сценарий В: Критическая системная ошибка (сетевой сбой, падение диска)
		// Идиоматично возвращаем нулевое значение строки "" и саму ошибку
		return "", err
	}

	// Если за maxShortenRetries попыток база данных так и не смогла принять запись
	return "", errors.New("Исчерпано максимальное количество попыток сохранения сайта в репозиторий")
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
