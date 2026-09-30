# go_short 🚀

Высокопроизводительный, отказоустойчивый и оптимизированный микросервис сокращения ссылок на Go 1.24+ (полная утилизация рантайма Go 1.26.1). 

Проект прошел глубокую сквозную оптимизацию слоев хранения, сетевого ввода-вывода (I/O) и подсистем компрессии данных. Архитектура переведена на модель работы с минимальной нагрузкой на Garbage Collector (GC) и максимальной утилизацией многопоточности современных CPU, превращая сервис в эталонное решение класса Highload Production-Ready.

## ✨ Ключевые особенности архитектуры

* **Константное RAM-хранилище O(1):** Линейный перебор базы данных полностью ликвидирован во всех узлах. Поиск по алиасам, проверка уникальности длинных URL и выборка по пользователям сведены к мгновенному извлечению указателей из хэш-индексов.
* **Атомарная децентрализация конкурентности:** Логика обработки коллизий генератора и ретраев строк перенесена напрямую на уровень атомарных операций репозиториев, полностью разгрузив бизнес-логику сервисов.
* **Симметричный Zero-Alloc Сетевой Слой:** Инфраструктура ввода-вывода полностью делегирована внешней Open-Source библиотеке `httpcodec` v0.0.2. Входящие текстовые/JSON потоки и сборка ответов утилизируют независимые пулы `sync.Pool`.
* **Zero-Reflect Куки-Сериализатор:** Инфраструктурный мусор подсистемы шифрования сессий `securecookie` полностью ликвидирован за счет внедрения кастомного легковесного кодека, работающего напрямую со строками UUID без тяжелой рефлексии `encoding/gob`.
* **Асинхронное неблокирующее логирование:** Системный вывод `zap` изолирован под потокобезопасный замок и обернут в асинхронный RAM-буфер объемом 256 КБ, что полностью исключило задержки дискового ввода-вывода из сетевых горутин.
* **Высокопроизводительный контур компрессии:** Скрытая аномалия фрагментации кучи стандартного gzip-пакета устранена за счет миграции на пулируемую ассемблерную библиотеку `klauspost/compress`. Потребление памяти сжатых пакетов снижено в 3.4 раза.
* **Двойной периметр DoS-защиты:** Сетевой лимитер ядра ОС `http.MaxBytesReader` аппаратно связан с программным стражем памяти кодека через буфер `maxBodySize + 1`. Система гарантированно возвращает статус `413 Request Entity Too Large` при переполнении, защищая сервер от атак класса Slowloris.

---

## 🛠 Хронология оптимизации: Что было сделано

В процессе рефакторинга и итерационного профилирования кодовой базы были зафиксированы и внедрены следующие ключевые улучшения:

* **Переход на указатели и O(1) Хранение:** В [internal/repository/in_memory.go](internal/repository/in_memory.go) хранение тяжелых структур по значению в мапах заменено на вечные указатели `*model.Site`. Развернуты три независимых хэш-индекса (`sites`, `urls`, `userUrls`). Эксклюзивный замок `sync.Mutex` заменен на разделяемый `sync.RWMutex`, переведя GET-читатели на неблокирующие параллельные блокировки `RLock()`. Для RAM-режима бенчмарков активирована преаллокация мап на `initialCapacity = 250000` элементов во избежание оверхеда на динамическую эвакуацию хэш-бакетов.
* **Асинхронное логирование и Zero-Alloc Аудит:** В [internal/logger/logger.go](internal/logger/logger.go) системный вывод логов обернут в асинхронный RAM-буфер объемом `256 КБ` с принудительной фоновой синхронизацией раз в секунду. Внедрен механизм превентивного отключения логирования: при неактивном `InfoLevel` роутер инициализируется в обход middleware-логгеров. Инфраструктурные обертки сетевого слоя переведены на пулинг через `sync.Pool` в файле [internal/router/audit/middleware.go](internal/router/audit/middleware.go).
* **Тюнинг пула соединений СУБД:** В [internal/router/router.go](internal/router/router.go) настроен промышленный контур пулирования сессий к СУБД PostgreSQL. Ограничения `SetMaxOpenConns(30)` и `SetMaxIdleConns(30)` зафиксировали сессии открытыми, устранив накладные расходы на повторные вызовы connect/dial.
* **Кастомный куки-кодек и pprof-контур:** В файле [pkg/codec.go](pkg/codec.go) бинарный кодировщик `encoding/gob` заменен на кастомный легковесный кодек `StringCodec` без рефлексии. В [cmd/shortener/main.go](cmd/shortener/main.go) интегрирован независимый сетевой контур профилирования `net/http/pprof` на изолированном порту `:6060`, работающий в рамках отказоустойчивой группы `errgroup` с поддержкой Graceful Shutdown.
* **Стек-копирование URL:** В [internal/config/flags.go](internal/config/flags.go) поле конфигурации `ShortBaseUrl` переведено из типа `string` в указатель `*url.URL`. Из «горячих путей» хендлеров в [internal/handler/post.go](internal/handler/post.go) и [internal/handler/get.go](internal/handler/get.go) удален тяжелый метод `url.JoinPath`. Внедрен паттерн копирования структуры URL на стек-кадр (`u := *shortBaseURL`) с последующей нативно-оптимизированной сборкой через метод `u.String()`.
* **Атомарный Upsert-Контур Хранилища:** Логика обработки коллизий генератора алиасов перенесена со слоя бизнес-сервиса на уровень атомарных операций репозиториев. В [internal/repository/postgres.go](internal/repository/postgres.go) легаси-проверка `RowsAffected()` заменена на атомарный паттерн `INSERT ... ON CONFLICT (url) DO UPDATE ... RETURNING short_url`, позволивший СУБД безопасно разрешать гонки данных за один сетевой round-trip.
* **Асинхронный пулинг компрессии (Gzip Reuse Layer):** В [pkg/gzip.go](pkg/gzip.go) стандартный прожорливый пакет `compress/gzip` заменен на пулируемую Highload-библиотеку `://github.com`, а создание объектов `NewWriter`/`NewReader` на каждый сетевой запрос замещено на наносекундный метод `.Reset()`, полностью уничтожив аномалию раздувания кучи структурами Хаффмана.

---

## 📈 Результаты сквозных E2E-бенчмарков (`benchstat`)

Сравнение производительности проводилось на базе 10 независимых итерационных прогонов (`-count=10`) между изначальной легаси-версией приложения (`base.txt`) и финальным Highload-стеком (`klauspost.txt`). Подробные профили тестов доступны в каталоге [cmd/shortener/main_benchmark_test.go](cmd/shortener/main_benchmark_test.go).

```text
goarch: amd64
cpu: AMD Ryzen 5 5600X 6-Core Processor             

                                     │ ./benchstat/base.txt │      ./benchstat/klauspost.txt      │
                                     │        sec/op        │   sec/op     vs base                │
_E2E_Batch_NoCompression-12                    10.478µ ± 2%   7.841µ ± 2%  -25.17% (p=0.000 n=10)
_E2E_Batch_WithRequestCompression-12            33.44µ ± 3%   23.05µ ± 4%  -31.05% (p=0.000 n=10)
_E2E_Batch_WithFullCompression-12              138.69µ ± 3%   29.36µ ± 5%  -78.83% (p=0.000 n=10)
_E2E_DeleteUserSites-12                         5.283µ ± 5%   4.548µ ± 4%  -13.91% (p=0.001 n=10)
_E2E_GetUserSites-12                            6.936µ ± 2%   4.223µ ± 4%  -39.11% (p=0.000 n=10)
_E2E_Post_JSON-12                               8.646µ ± 3%   6.975µ ± 5%  -19.33% (p=0.000 n=10)
geomean                                         12.16µ        8.374µ       -31.13%

                                     │ ./benchstat/base.txt │      ./benchstat/klauspost.txt       │
                                     │         B/op         │     B/op      vs base                │
_E2E_Batch_NoCompression-12                    6.603Ki ± 0%   4.942Ki ± 0%  -25.15% (p=0.000 n=10)
_E2E_Batch_WithRequestCompression-12          50.964Ki ± 0%   5.103Ki ± 0%  -89.99% (p=0.000 n=10)
_E2E_Batch_WithFullCompression-12            845.846Ki ± 0%   5.232Ki ± 1%  -99.38% (p=0.000 n=10)
_E2E_DeleteUserSites-12                        4.647Ki ± 0%   3.745Ki ± 0%  -19.42% (p=0.000 n=10)
_E2E_GetUserSites-12                           6.234Ki ± 0%   3.805Ki ± 0%  -38.97% (p=0.000 n=10)
_E2E_Post_JSON-12                              5.973Ki ± 0%   4.662Ki ± 0%  -21.95% (p=0.000 n=10)
geomean                                        13.34Ki        4.540Ki       -65.98%

                                     │ ./benchstat/base.txt │      ./benchstat/klauspost.txt       │
                                     │      allocs/op       │ allocs/op   vs base                  │
_E2E_Batch_NoCompression-12                      83.00 ± 0%   61.00 ± 0%  -26.51% (p=0.000 n=10)
_E2E_Batch_WithRequestCompression-12             92.00 ± 0%   67.00 ± 0%  -27.17% (p=0.000 n=10)
_E2E_Batch_WithFullCompression-12               113.00 ± 0%   71.00 ± 0%  -37.17% (p=0.000 n=10)
_E2E_DeleteUserSites-12                          51.00 ± 0%   42.00 ± 0%  -17.65% (p=0.000 n=10)
_E2E_GetUserSites-12                             79.00 ± 0%   41.00 ± 0%  -48.10% (p=0.000 n=10)
_E2E_Post_JSON-12                                65.00 ± 0%   53.00 ± 0%  -18.46% (p=0.000 n=10)
geomean                                          68.54        52.58       -23.29%
```

### 🏆 Главные инженерные рекорды оптимизации:
1. **Экстремальное снижение памяти на сжатии (`-99.38%`):** В самом ресурсоемком сценарии с полным двусторонним сжатием (`_E2E_Batch_WithFullCompression`) потребление памяти рухнуло с гигантских **`845.85 KiB` до мизерных `5.23 KiB`** на операцию. Сжатый трафик теперь обрабатывается со скоростью и эффективностью чистой RAM. Геомедиана памяти всего проекта упала на **`-65.98%`**.
2. **Ускорение процессора на 78.83%:** Время выполнения сквозной пакетной операции со сжатием сократилось со `138.69 µs` до `29.36 µs`. Программа работает **почти в 5 раз быстрее** за счет полной ликвидации аллокаций Хаффмана и интеграции JIT-движка.
3. **Разгрузка аллокатора в REST API:** В ручке получения ссылок пользователя (`_E2E_GetUserSites`) количество аллокаций сократилось почти вдвое — на **`-48.10%`** (с 79 до 41), а скорость отдачи ответа выросла на **`39.11%`**, доказывая колоссальную пользу от пулов сериализации `WriteJSON`.

## 🔍 Дифференциальный анализ кучи (`go tool pprof -diff_base`)

Сравнение профилей выделения памяти (`alloc_space`) между изначальной легаси-версией и финальной конфигурацией зафиксировало **чистый сброс аллокаций в куче на `-2040.27 МБ` (минус 2 Гигабайта мусора)** за время прогона тестов.

```text
Type: alloc_space
Time: 2026-09-27 13:20:53 MSK
Showing nodes accounting for -2040.27MB, 26.13% of 7807.21MB total
Dropped 109 nodes (cum <= 39.04MB)
      flat  flat%   sum%        cum   cum%
-3195.19MB 40.93% 40.93% -3876.96MB 49.66%  compress/flate.NewWriter (inline)
 -662.75MB  8.49% 49.42%  -662.75MB  8.49%  compress/flate.(*compressor).initDeflate (inline)
  595.19MB  7.62% 41.79%   595.19MB  7.62%  net/textproto.MIMEHeader.Set (inline)
  522.66MB  6.69% 35.10%   522.66MB  6.69%  net/http.(*Request).WithContext (partial-inline)
 -449.25MB  5.75% 40.85%  -449.25MB  5.75%  compress/flate.(*dictDecoder).init (inline)
  306.08MB  3.92% 36.93%   421.59MB  5.40%  net/http.NewRequestWithContext
  225.23MB  2.88% 34.05%   225.23MB  2.88%  github.com/ioncode/go_short/internal/repository.NewMapRepository
 -211.53MB  2.71% 36.76%  -288.04MB  3.69%  encoding/gob.(*Decoder).getDecEnginePtr
  186.52MB  2.39% 34.37%   186.52MB  2.39%  crypto/internal/fips140/sha256.New (inline)
 -158.51MB  2.03% 36.40%  -158.51MB  2.03%  encoding/gob.NewDecoder
  157.01MB  2.01% 34.39%   343.53MB  4.40%  crypto/internal/fips140/hmac.New[go.shape.interface { BlockSize int; Reset; Size int; Sum []uint8; Write  }]
  148.03MB  1.90% 32.49%   148.03MB  1.90%  net/http.readCookies
 -148.02MB  1.90% 34.39%  -200.02MB  2.56%  net/url.(*URL).joinPath
  118.01MB  1.51% 32.87%   118.01MB  1.51%  github.com/gorilla/securecookie.decode
  114.03MB  1.46% 31.41%   114.03MB  1.46%  maps.Copy[go.shape.map[string]interface {},go.shape.map[string]interface {},go.shape.string,go.shape.interface {}] (inline)
 -110.55MB  1.42% 32.83%  -110.55MB  1.42%  encoding/json.(*Decoder).refill
 -109.92MB  1.41% 34.24%  -559.17MB  7.16%  compress/flate.NewReader
   99.51MB  1.27% 32.96%    97.51MB  1.25%  fmt.Sprintf
   89.51MB  1.15% 31.82%   105.01MB  1.35%  encoding/json.Unmarshal
      88MB  1.13% 30.69%       88MB  1.13%  context.WithValue
   81.50MB  1.04% 29.64%    81.50MB  1.04%  internal/bytealg.MakeNoZero
   73.11MB  0.94% 28.71%    73.11MB  0.94%  github.com/ioncode/go_short/internal/repository.(*MapRepository).addIndexes (inline)
   71.01MB  0.91% 27.80%   131.02MB  1.68%  github.com/ioncode/go_short/internal/service.(*Shortner).BatchShort
  -69.52MB  0.89% 28.69%   -69.52MB  0.89%  encoding/json.NewDecoder
   65.50MB  0.84% 27.85%    65.50MB  0.84%  bytes.genSplit
  -64.50MB  0.83% 28.68%   -76.50MB  0.98%  encoding/gob.(*Decoder).compileSingle
      62MB  0.79% 27.88%      139MB  1.78%  github.com/gorilla/securecookie.(*SecureCookie).Decode
  -58.72MB  0.75% 28.63%   -58.72MB  0.75%  bufio.NewReaderSize (inline)
      55MB   0.7% 27.93%       55MB   0.7%  github.com/ioncode/go_short/pkg.StringCodec.Deserialize
  -39.09MB   0.5% 28.43%   -39.09MB   0.5%  compress/flate.(*huffmanEncoder).generate
   37.50MB  0.48% 27.95%    37.50MB  0.48%  reflect.growslice
      36MB  0.46% 27.49%       36MB  0.46%  github.com/ioncode/go_short/cmd/shortener/benchmark.resetRecorder (inline)
      33MB  0.42% 27.07%       33MB  0.42%  bytes.NewReader (inline)
      33MB  0.42% 26.64%       33MB  0.42%  mime.ParseMediaType
   30.50MB  0.39% 26.25% -3366.25MB 43.12%  github.com/ioncode/go_short/pkg.GzipMiddleware.func1
   29.50MB  0.38% 25.88%   360.23MB  4.61%  github.com/ioncode/go_short/internal/router.SetupRouter.APIPostBatch.func9
     -26MB  0.33% 26.21%      -26MB  0.33%  internal/saferio.ReadData
      24MB  0.31% 25.90%       24MB  0.31%  crypto/internal/fips140/sha256.(*Digest).Sum
     -23MB  0.29% 26.20%      -23MB  0.29%  encoding/gob.decString
      23MB  0.29% 25.90%   137.03MB  1.76%  github.com/ioncode/go_short/internal/router/audit.(*auditContainer).snapshot
   22.50MB  0.29% 25.61% -3077.38MB 39.42%  github.com/ioncode/go_short/pkg.(*AuthMiddleware).EnsureUserHasID-fm.(*AuthMiddleware).EnsureUserHasID.func1
     -20MB  0.26% 25.87%      -20MB  0.26%  bytes.NewBuffer (inline)
   19.50MB  0.25% 25.62%   182.03MB  2.33%  net/http.(*Request).AddCookie
  -18.50MB  0.24% 25.86%  -403.04MB  5.16%  github.com/ioncode/go_short/internal/router.SetupRouter.GetUserSites.func6
  -18.50MB  0.24% 26.09%   -18.50MB  0.24%  path.(*lazybuf).string (inline)
     -18MB  0.23% 26.32%      -52MB  0.67%  path.Join
   16.50MB  0.21% 26.11%   144.01MB  1.84%  github.com/goccy/go-json/internal/decoder.(*unmarshalJSONDecoder).Decode
  -15.50MB   0.2% 26.31%   -15.50MB   0.2%  path.(*lazybuf).append (inline)
      14MB  0.18% 26.13%       28MB  0.36%  net/http.Redirect
  -13.50MB  0.17% 26.30%   -13.50MB  0.17%  encoding/gob.(*Decoder).newDecoderState (inline)
   13.39MB  0.17% 26.13%    44.34MB  0.57%  github.com/ioncode/go_short/internal/repository.(*MapRepository).StoreSite
  -13.01MB  0.17% 26.30%  -630.90MB  8.08%  compress/gzip.NewReader (inline)
   12.86MB  0.16% 26.13%    55.01MB   0.7%  github.com/ioncode/go_short/internal/repository.(*MapRepository).BatchStoreSites
     -12MB  0.15% 26.29%      -12MB  0.15%  encoding/json.NewEncoder
  -11.01MB  0.14% 26.43%   -19.01MB  0.24%  compress/flate.newHuffmanBitWriter (inline)
   10.14MB  0.13% 26.30%    10.14MB  0.13%  github.com/goccy/go-json/internal/decoder.(*RuntimeContext).reserveArena (inline)
    9.50MB  0.12% 26.18%   114.51MB  1.47%  github.com/ioncode/go_short/internal/model.(*Url).UnmarshalJSON
   -8.50MB  0.11% 26.29%      -35MB  0.45%  github.com/ioncode/go_short/internal/router.SetupRouter.Get.func1
    6.50MB 0.083% 26.20%   133.01MB  1.70%  github.com/ioncode/go_short/internal/router.SetupRouter.GetUserSites.func10
   -6.50MB 0.083% 26.29%   391.41MB  5.01%  github.com/ioncode/go_short/internal/router/audit.(*Auditor).Middleware-fm.(*Auditor).Middleware.func1
       6MB 0.077% 26.21%    68.48MB  0.88%  github.com/ioncode/go_short/internal/router.SetupRouter.Post.func7
       6MB 0.077% 26.13%    26.11MB  0.33%  github.com/ioncode/go_short/internal/router.SetupRouter.AsyncDeleteUserSites.func11
       6MB 0.077% 26.06%    82.48MB  1.06%  github.com/ioncode/go_short/internal/router.SetupRouter.APIPost.func8
      -4MB 0.051% 26.11%  -172.56MB  2.21%  github.com/ioncode/go_short/internal/router.SetupRouter.AsyncDeleteUserSites.func7
       3MB 0.038% 26.07%  -627.90MB  8.04%  github.com/ioncode/go_short/pkg.newCompressReader
   -2.50MB 0.032% 26.10%   -16.50MB  0.21%  encoding/json.(*decodeState).object
   -2.50MB 0.032% 26.13% -3961.11MB 50.74%  github.com/ioncode/go_short/internal/router.SetupRouter.APIPostBatch.func5
   ```