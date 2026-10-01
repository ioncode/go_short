package repository

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/ioncode/go_short/internal/model"
)

// Ошибки бизнес-логики репозитория.
var (
	ErrSiteExists    = errors.New("Site allready shorted")
	ErrSiteNotFound  = errors.New("Site not found")
	ErrAliasConflict = errors.New("Alias allready generated, try new one")
)

// MapRepository — это потокобезопасная реализация хранилища сокращенных ссылок в оперативной памяти (RAM).
//
// Структура оптимизирована для высоких нагрузок и честного E2E-профилирования:
//   - Использует sync.RWMutex для параллельного неблокирующего чтения (GET запросы).
//   - Хранит указатели *model.Site для экономии памяти и исключения дублирования строк.
//   - Содержит три хэш-индекса для мгновенного доступа к данным за константное время O(1).
type MapRepository struct {
	sites    map[model.ShortUrl]*model.Site // Индекс для быстрого поиска по короткому алиасу (O(1))
	urls     map[model.Url]*model.Site      // Индекс для мгновенной проверки уникальности оригинального URL (O(1))
	userUrls map[uuid.UUID][]model.ShortUrl // Индекс для мгновенного получения ссылок конкретного пользователя (O(1))
	mutex    sync.RWMutex                   // RWMutex для безопасного конкурентного доступа к мапам
	file     *os.File                       // Дескриптор файла для персистентного хранения данных на диске
}

// NewMapRepository создает и инициализирует новый экземпляр MapRepository.
//
// Если storagePath равен пустой строке, репозиторий инициализируется в режиме
// чистой оперативной памяти (RAM) с преаллокацией мап на 250 000 элементов,
// что оптимизирует работу E2E-бенчмарков и устраняет оверхед на динамическое расширение.
//
// Если storagePath указан, функция открывает файл, вычитывает его содержимое
// и последовательно восстанавливает состояние всех внутренних индексов.
func NewMapRepository(storagePath string) *MapRepository {
	// Константа преаллокации емкости мап для RAM-режима (бенчмарков)
	const initialCapacity = 250000

	if storagePath == "" {
		return &MapRepository{
			sites:    make(map[model.ShortUrl]*model.Site, initialCapacity),
			urls:     make(map[model.Url]*model.Site, initialCapacity),
			userUrls: make(map[uuid.UUID][]model.ShortUrl),
			file:     nil,
		}
	}

	file, err := os.OpenFile(storagePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalln("Storage path not opened", err, storagePath)
	}

	repository := MapRepository{
		sites:    make(map[model.ShortUrl]*model.Site),
		urls:     make(map[model.Url]*model.Site),
		userUrls: make(map[uuid.UUID][]model.ShortUrl),
		file:     file,
	}

	reader := bufio.NewReader(file)
	decoder := json.NewDecoder(reader)
	t, err := decoder.Token()
	if err != nil && err != io.EOF {
		log.Fatalf("Failed to read token: %v", err)
	}

	if t != nil {
		for decoder.More() {
			// аллоцируем сайт в куче и получаем вечный указатель
			sitePtr := new(model.Site)
			err := decoder.Decode(sitePtr)
			if err != nil {
				log.Fatalf("Failed to decode site : %v", err)
			}

			// раскидываем указатель по индексам
			repository.addIndexes(sitePtr)
		}
	}

	return &repository
}

// addIndexes является внутренним неэкспортируемым методом для атомарного
// добавления указателя на сайт во все три внутренние мапы-индексы.
//
// ВНИМАНИЕ: Метод thread-unsafe и должен вызываться строго из-под блокировки r.mutex.
func (r *MapRepository) addIndexes(sitePtr *model.Site) {
	r.sites[sitePtr.ShortUrl] = sitePtr
	r.urls[sitePtr.Url] = sitePtr
	r.userUrls[sitePtr.UserId] = append(r.userUrls[sitePtr.UserId], sitePtr.ShortUrl)
}

// GetByAlias возвращает информацию о сайте по его короткому буквенно-цифровому алиасу.
//
// Метод использует разделяемую блокировку RLock, позволяя неограниченному числу
// параллельных запросов на чтение (GET) выполняться без задержек.
// Если алиас найден, возвращается копия структуры model.Site. Если алиас
// отсутствует в базе, возвращается пустая структура и ошибка ErrSiteNotFound.
func (r *MapRepository) GetByAlias(alias model.ShortUrl) (model.Site, error) {
	r.mutex.RLock() // Разрешаем параллельное конкурентное чтение
	defer r.mutex.RUnlock()

	sitePtr, ok := r.sites[alias]
	if ok {
		// Разыменовываем указатель, возвращая изолированную копию структуры во внешний код
		return *sitePtr, nil
	}
	return model.Site{}, ErrSiteNotFound
}

// StoreSite атомарно сохраняет информацию о сокращенном сайте в RAM-репозиторий.
//
// Метод полностью потокобезопасен и оптимизирован для конкурентной среды:
//   - Захватывает эксклюзивную блокировку (Lock) на время проверки индексов и вставки.
//   - За константное время O(1) проверяет уникальность сгенерированного ShortUrl.
//   - За константное время O(1) проверяет, не был ли оригинальный Url сокращен ранее.
//
// Если оригинальный Url уже присутствует в системе, метод возвращает его существующий
// короткий алиас и ошибку-маркер ErrSiteExists.
// После успешного обновления RAM-индексов данные асинхронно сбрасываются в файл,
// если для репозитория задан файловый путь персистентности.
//
// Параметры:
//   - site: структура model.Site с оригинальным URL, сгенерированным алиасом и ID пользователя.
//
// Возвращаемые значения:
//   - model.ShortUrl: итоговый алиас (новый сгенерированный или уже существующий в базе).
//   - error: nil при успешном сохранении; ErrAliasConflict, если сгенерированная строка
//     совпала с чужим алиасом; ErrSiteExists, если длинный URL уже сокращен;
//     системная ошибка при сбое записи на диск.
func (r *MapRepository) StoreSite(site model.Site) (model.ShortUrl, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// 1. Проверяем коллизию сгенерированного короткого алиаса по индексу sites за O(1).
	// Если такой буквенно-цифровой код уже занят другим сайтом, возвращаем ошибку коллизии,
	// чтобы сервис Shortner мог уйти на следующий круг генерации строки.
	if _, ok := r.sites[site.ShortUrl]; ok {
		return "", ErrAliasConflict
	}

	// 2. Быстрая проверка уникальности оригинального Url по хэш-индексу urls за O(1).
	// Если URL уже сокращен (в текущем или параллельном запросе), мы атомарно
	// извлекаем и возвращаем его старый алиас вместе с ошибкой ErrSiteExists.
	if existingSite, ok := r.urls[site.Url]; ok {
		return existingSite.ShortUrl, ErrSiteExists
	}

	// 3. Если проверки пройдены, создаем изолированную копию структуры в куче
	// и атомарно раскидываем указатели по всем трем внутренним мапам-индексам.
	allocatedSite := new(model.Site)
	*allocatedSite = site
	r.addIndexes(allocatedSite)

	// 4. Если персистентность отключена (RAM-режим для бенчмарков), завершаем операцию
	if r.file == nil {
		return site.ShortUrl, nil
	}

	// 5. Синхронизируем состояние RAM-памяти с диском.
	// В случае ошибки записи возвращаем пустую строку и ошибку ввода-вывода.
	if err := r.flushToFile(); err != nil {
		return "", err
	}

	return site.ShortUrl, nil
}

// BatchStoreSites выполняет пакетное сохранение среза сайтов в репозиторий.
//
// Метод полностью оптимизирован под рантайм. Чтобы избежать утечек памяти
// и удержания в куче всего входящего среза сайтов, для каждого элемента создается
// независимая копия структуры в куче. Указатели на эти копии сохраняются во все
// внутренние индексы за O(1) на элемент.
func (r *MapRepository) BatchStoreSites(sites []model.Site) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for i := range sites {
		// Создаем изолированную копию структуры в куче.
		// Это гарантирует, что временный массив входящего среза sites будет успешно
		// уничтожен сборщиком мусора сразу после завершения HTTP-запроса.
		allocatedSite := new(model.Site)
		*allocatedSite = sites[i] // Копируем данные

		// Передаем адрес безопасной изолированной копии во все индексы за O(1)
		r.addIndexes(allocatedSite)
	}

	if r.file == nil {
		return nil
	}
	return r.flushToFile()
}

// GetByUrl производит поиск сохраненного сайта по его полному оригинальному URL.
//
// БЫЛО: Полный перебор мапы в цикле со сложностью O(N).
// СТАЛО: Мгновенное извлечение значения из хэш-индекса r.urls за время O(1).
// Метод защищен RLock, что исключает деградацию производительности E2E-бенчмарков
// на этапе проверки дубликатов в сервисе сокращателя ссылок.
func (r *MapRepository) GetByUrl(url model.Url) (model.Site, error) {
	r.mutex.RLock() // Разрешаем параллельное конкурентное чтение
	defer r.mutex.RUnlock()

	// Мгновенный поиск по хэш-мапе вместо тяжелого цикла
	sitePtr, ok := r.urls[url]
	if ok {
		return *sitePtr, nil
	}
	return model.Site{}, ErrSiteNotFound
}

// Close закрывает дескриптор файла, если включен режим персистентного хранения.
func (r *MapRepository) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}

// Ping проверяет доступность репозитория.
//
// Для MapRepository, работающего полностью в оперативной памяти (RAM),
// данная операция всегда успешна и возвращает nil, за исключением случаев,
// когда переданный контекст ctx уже отменен или истек по таймауту.
func (r *MapRepository) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err // Уважаем отмену контекста внешней системой мониторинга
	}
	return nil // RAM-репозиторий всегда доступен, пока работает процесс
}

// GetByUser возвращает список всех сокращенных ссылок, принадлежащих конкретному пользователю.
//
// БЫЛО: Линейный поиск O(N), который сканировал всю базу данных при каждом запросе.
// СТАЛО: Извлечение готового среза альясов из индекса r.userUrls за O(1).
// Если у пользователя нет ссылок, метод возвращает (nil, nil) без аллокаций памяти.
func (r *MapRepository) GetByUser(authorID uuid.UUID) ([]model.UserSitesResponseItem, error) {
	r.mutex.RLock() // Разрешаем параллельное конкурентное чтение
	defer r.mutex.RUnlock()

	aliases, ok := r.userUrls[authorID]
	if !ok || len(aliases) == 0 {
		return nil, nil // 0 аллокаций памяти при отсутствии данных у пользователя
	}

	// Аллоцируем срез сразу нужной емкости, избегая динамических расширений в цикле
	result := make([]model.UserSitesResponseItem, 0, len(aliases))
	for _, alias := range aliases {
		if sitePtr, exists := r.sites[alias]; exists {
			result = append(result, model.UserSitesResponseItem{
				Alias: sitePtr.ShortUrl,
				URL:   sitePtr.Url,
			})
		}
	}
	return result, nil
}

// Delete выполняет асинхронное «мягкое» удаление (soft delete) пакета сокращенных ссылок.
//
// Метод итерируется по переданному срезу алиасов, проверяет права владения (UserId)
// и выставляет флаг DeletedFlag = true непосредственно внутри структуры сайта.
// Операция защищена эксклюзивным Mutex, так как происходит модификация данных.
// Если в процессе выполнения контекст ctx отменяется или истекает его таймаут,
// метод немедленно прерывает операцию и возвращает ошибку контекста.
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

		// Поскольку r.sites хранит указатели, мы модифицируем флаг
		// напрямую в объекте кучи без необходимости перезаписывать мапу
		if sitePtr, exists := r.sites[alias]; exists && sitePtr.UserId == user.ID && !sitePtr.DeletedFlag {
			sitePtr.DeletedFlag = true
			changed = true
		}
	}

	// Синхронизируем файл на диске только в случае реального изменения флагов
	if changed {
		if r.file == nil {
			return nil
		}
		return r.flushToFile()
	}
	return nil
}

// flushToFile перезаписывает файл текущим состоянием r.sites.
// Должен вызываться под заблокированным r.mutex.
func (r *MapRepository) flushToFile() error {
	if r.file == nil {
		return nil
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
	allSites := make([]*model.Site, 0, len(r.sites))
	for _, sitePtr := range r.sites {
		allSites = append(allSites, sitePtr)
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(allSites); err != nil {
		return err
	}
	return writer.Flush()
}
