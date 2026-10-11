package service

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
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
	BatchStoreSites(ctx context.Context, sites []model.Site, genAlias func() model.ShortUrl) error
	GetByUser(authorID uuid.UUID) ([]model.UserSitesResponseItem, error)
	Delete(ctx context.Context, aliases []model.ShortUrl, user model.User) error
}

type DeleteTask struct {
	Author  model.User
	Aliases []model.ShortUrl
}

// service struct
type Shortner struct {
	repository SiteRepository
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
	return "", errors.New("исчерпано максимальное количество попыток сохранения сайта в репозиторий")
}

// BatchShort выполняет высокопроизводительное пакетное сокращение слайса URL.
//
// Метод полностью lock-free (s.mutex удален) и работает параллельно для всех ядер CPU.
// Логика атомарного разрешения межзапросных гонок (Race Condition) и коллизий токенов
// делегирована на уровень репозитория под его внутренний механизм изоляции.
func (s *Shortner) BatchShort(ctx context.Context, items []model.BatchPostRequestItem, user model.User) ([]model.BatchPostResponseItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// 1. Преаллокация памяти — строго под размер входящей пачки.
	// Это гарантирует ровно одну аллокацию в куче на всю функцию (Zero-Alloc в цикле).
	newSites := make([]model.Site, len(items))
	for i := range items {
		// Генерируем случайный текстовый идентификатор оптимистично на стеке горутины
		alias := model.ShortUrl(stringWithCharset(8))

		newSites[i] = model.Site{
			CorrelationId: items[i].CorrelationId,
			Url:           items[i].URL,
			ShortUrl:      alias,
			UserId:        user.ID,
		}
	}

	// 2. Передаем функцию генерации как легковесное замыкание (Слои архитектуры сохраняются).
	// Репозиторий вызовет её как «черный ящик» только в случае редчайшего совпадения токенов в базе.
	gen := func() model.ShortUrl {
		return model.ShortUrl(stringWithCharset(8))
	}

	// 3. Отправляем ВСЮ пачку в репозиторий за один атомарный вызов.
	// Метод репозитория под своей блокировкой (или транзакцией СУБД с EXCLUDED.correlation_id)
	// проверит дубликаты и изменит ShortUrl прямо внутри переданного среза newSites.
	err := s.repository.BatchStoreSites(ctx, newSites, gen)
	if err != nil {
		return nil, err
	}

	// 4. Формируем финальный ответ клиенту на основе гарантированно консистентных данных.
	// newSites[i].ShortUrl теперь содержит либо новый сгенерированный алиас,
	// либо старый алиас, который ранее выиграл конкурентную гонку в базе данных.
	responseItems := make([]model.BatchPostResponseItem, len(items))
	for i := range newSites {
		responseItems[i] = model.BatchPostResponseItem{
			CorrelationId: newSites[i].CorrelationId,
			Alias:         newSites[i].ShortUrl,
		}
	}

	return responseItems, nil
}

func (s *Shortner) GetByUser(authorID uuid.UUID) ([]model.UserSitesResponseItem, error) {
	return s.repository.GetByUser(authorID)
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
