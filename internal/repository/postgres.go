package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ioncode/go_short/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresSitesRepository struct {
	db *sql.DB
}

func NewPostgresSitesRepository(db *sql.DB) *PostgresSitesRepository {
	return &PostgresSitesRepository{db: db}
}

func (r *PostgresSitesRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *PostgresSitesRepository) GetByAlias(alias model.ShortUrl) (model.Site, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	row := r.db.QueryRowContext(ctx,
		"SELECT url, is_deleted "+
			"FROM sites WHERE short_url = $1 LIMIT 1", alias)

	var site model.Site
	err := row.Scan(&site.Url, &site.DeletedFlag)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return site, ErrSiteNotFound
		}
		return site, fmt.Errorf("postgres: get site by alias %q: %w", alias, err)
	}

	site.ShortUrl = alias
	return site, nil
}

func (r *PostgresSitesRepository) GetByUrl(url model.Url) (model.Site, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	row := r.db.QueryRowContext(ctx,
		"SELECT short_url "+
			"FROM sites WHERE url = $1 LIMIT 1", url)

	var site model.Site
	err := row.Scan(&site.ShortUrl)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return site, ErrSiteNotFound
		}
		return site, fmt.Errorf("postgres: get site by URL %q: %w", url, err)
	}

	site.Url = url
	return site, nil
}

func (r *PostgresSitesRepository) GetByUser(authorID uuid.UUID) ([]model.UserSitesResponseItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	records := []model.UserSitesResponseItem{}

	query := `SELECT url, short_url FROM sites WHERE user_id = $1`
	rows, err := r.db.QueryContext(ctx, query, authorID)
	if err != nil {
		return records, err
	}
	defer func() { _ = rows.Close() }()

	// Читаем строки из БД
	for rows.Next() {
		var rec model.UserSitesResponseItem
		err := rows.Scan(&rec.URL, &rec.Alias)
		if err != nil {
			return records, err
		}
		records = append(records, rec)
	}

	// Проверяем, не возникло ли ошибок при итерации
	if err = rows.Err(); err != nil {
		return records, err
	}

	return records, nil
}

func (r *PostgresSitesRepository) StoreSite(site model.Site) (model.ShortUrl, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Направляем ON CONFLICT строго на индекс оригинального URL (idx_sites_url).
	// Если URL уже существует, мы делаем фиктивный апдейт (url = SITES.url),
	// чтобы сработал RETURNING и вернул нам старый short_url.
	query := `
		INSERT INTO SITES (url, short_url, user_id) 
		VALUES ($1, $2, $3)
		ON CONFLICT (url) DO UPDATE SET url = SITES.url
		RETURNING short_url;
	`

	var resultAlias string
	err := r.db.QueryRowContext(ctx, query, site.Url, site.ShortUrl, site.UserId).Scan(&resultAlias)

	if err != nil {
		// Проверяем, не является ли ошибка результатом коллизии сгенерированного short_url
		var pgErr *pgconn.PgError // Если используете jackc/pgx/v5
		if errors.As(err, &pgErr) {
			// Код ошибки 23505 — unique_violation
			if pgErr.Code == "23505" {
				// Если имя нарушенного индекса/ограничения содержит short_url,
				// значит, наш генератор случайно выдал уже существующий в базе токен.
				if strings.Contains(pgErr.ConstraintName, "short_url") {
					return "", ErrAliasConflict // Возвращаем маркер, сервис уйдет на ретрай
				}
			}
		}
		return "", fmt.Errorf("postgres: store site error: %w", err)
	}

	// Если возвращенный из базы алиас НЕ совпадает с тем, который мы генерировали,
	// значит сработал триггер ON CONFLICT, и этот URL уже существовал в системе.
	if resultAlias != string(site.ShortUrl) {
		return model.ShortUrl(resultAlias), ErrSiteExists
	}

	// Успешная вставка новой уникальной ссылки
	return site.ShortUrl, nil
}

func (r *PostgresSitesRepository) Close() error {
	return r.db.Close()
}

func (r *PostgresSitesRepository) Delete(ctx context.Context, aliases []model.ShortUrl, user model.User) error {
	query := `
		UPDATE sites SET is_deleted = true 
		WHERE user_id = $1 
		AND short_url = ANY($2) 
		AND is_deleted = false;`
	// pgx/v5/stdlib прозрачно преобразует []model.ShortUrl в массив БД,
	// pq.Array(aliases) здесь больше писать НЕ НУЖНО.
	_, err := r.db.ExecContext(ctx, query, user.ID, aliases)
	return err
}

// BatchStoreSites выполняет пакетное сохранение сайтов с атомарным разрешением коллизий.
// Метод полностью lock-free на уровне Go-сервиса и использует бинарный протокол COPY.
func (r *PostgresSitesRepository) BatchStoreSites(ctx context.Context, sites []model.Site, genAlias func() model.ShortUrl) error {
	// Извлекаем нативное соединение из пула sql.DB
	pgxConn, err := r.extractPgxConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgxConn.Close(ctx) }()

	// Предвыделяем буфер под строки один раз на все ретраи (Zero-Alloc в цикле)
	rowsBuf := make([][]any, 0, len(sites))

	// цикл транзакций для обработки коллизий алиасов
	for {
		// Если во время прошлой коллизии токена у клиента истек таймаут,
		// мы прерываем цикл и не нагружаем Postgres лишними транзакциями.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("postgres: context canceled before retry: %w", err)
		}

		// Открываем транзакцию напрямую через pgx для изоляции батча от Race Conditions
		tx, err := pgxConn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("postgres: begin pgx tx error: %w", err)
		}

		// Выполняем весь пайплайн внутри текущей транзакции через изолированную функцию,
		// чтобы избежать утечек defer в бесконечном цикле.
		actualAliases, err := r.executeBatchPipeline(ctx, tx, pgxConn, sites, &rowsBuf)
		if err == nil {
			// УСПЕХ: прописываем выигравшие алиасы обратно в массив сервиса
			for i := range sites {
				if winnerAlias, exists := actualAliases[sites[i].CorrelationId]; exists {
					sites[i].ShortUrl = winnerAlias
				}
			}
			return tx.Commit(ctx)
		}

		// Всегда явно откатываем текущую транзакцию этой итерации при любой ошибке
		_ = tx.Rollback(ctx)

		// Декларативно проверяем, была ли ошибка коллизией случайно сгенерированного токена (short_url)
		if r.isShortUrlCollision(err) {
			// Перегенерируем алиасы на CPU и уходим на следующий круг цикла ретрая
			for i := range sites {
				sites[i].ShortUrl = genAlias()
			}
			continue
		}

		// Если ошибка критическая (сетевой сбой, права доступа) — прекращаем работу
		return fmt.Errorf("postgres: batch store fatal error: %w", err)
	}
}

// executeBatchPipeline координирует последовательность шагов внутри одной транзакции.
func (r *PostgresSitesRepository) executeBatchPipeline(
	ctx context.Context,
	tx pgx.Tx,
	pgxConn *pgx.Conn,
	sites []model.Site,
	rowsBuf *[][]any,
) (map[string]model.ShortUrl, error) {
	// Шаг 1: Создаем пустую стейджинг-таблицу
	if err := r.prepareStagingTable(ctx, tx); err != nil {
		return nil, err
	}

	// Шаг 2: Быстро заливаем данные через бинарный CopyFrom
	if err := r.copyToStaging(ctx, pgxConn, sites, rowsBuf); err != nil {
		return nil, err
	}

	// Шаг 3: Сливаем в основную таблицу с RETURNING по EXCLUDED.correlation_id
	actualAliases, err := r.mergeStagingToMain(ctx, tx)
	if err != nil {
		return nil, err
	}

	return actualAliases, nil
}

// extractPgxConn безопасно извлекает низкоуровневый *pgx.Conn из стандартного пула *sql.DB.
func (r *PostgresSitesRepository) extractPgxConn(ctx context.Context) (*pgx.Conn, error) {
	sqlConn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: get underlying conn error: %w", err)
	}

	var pgxConn *pgx.Conn
	err = sqlConn.Raw(func(driverConn any) error {
		if stdlibConn, ok := driverConn.(*stdlib.Conn); ok {
			pgxConn = stdlibConn.Conn()
			return nil
		}
		return errors.New("driver connection is not a pgx stdlib connection")
	})

	if err != nil {
		_ = sqlConn.Close()
		return nil, fmt.Errorf("postgres: extract pgx.Conn error: %w", err)
	}

	return pgxConn, nil
}

// prepareStagingTable создает временную таблицу без индексов для мгновенного приема батча.
func (r *PostgresSitesRepository) prepareStagingTable(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_sites ON COMMIT DROP AS 
		SELECT correlation_id, url, short_url, user_id FROM sites WITH NO DATA;
	`)
	if err != nil {
		return fmt.Errorf("prepare staging table failed: %w", err)
	}
	return nil
}

// copyToStaging выполняет бинарное копирование строк в стейджинг-таблицу, повторно используя буфер памяти.
func (r *PostgresSitesRepository) copyToStaging(
	ctx context.Context,
	pgxConn *pgx.Conn,
	sites []model.Site,
	rowsBuf *[][]any,
) error {
	// Сбрасываем длину буфера до нуля, полностью сохраняя его выделенную емкость (cap)
	*rowsBuf = (*rowsBuf)[:0]

	for i := range sites {
		*rowsBuf = append(*rowsBuf, []any{
			sites[i].CorrelationId,
			sites[i].Url,
			sites[i].ShortUrl,
			sites[i].UserId,
		})
	}

	// Вызываем CopyFrom у соединения pgxConn (протокол COPY берет сокет под контроль)
	_, err := pgxConn.CopyFrom(
		ctx,
		pgx.Identifier{"tmp_sites"},
		[]string{"correlation_id", "url", "short_url", "user_id"},
		pgx.CopyFromRows(*rowsBuf),
	)
	if err != nil {
		return fmt.Errorf("copyfrom operation failed: %w", err)
	}
	return nil
}

// mergeStagingToMain производит атомарное слияние с СУБД без мутации исторических пустых correlation_id.
func (r *PostgresSitesRepository) mergeStagingToMain(ctx context.Context, tx pgx.Tx) (map[string]model.ShortUrl, error) {
	// Оборачиваем INSERT в СТЕ (inserted_rows). Он возвращает только url и short_url.
	// Затем мы честно через JOIN соединяем результат с tmp_sites по уникальному url.
	// Это позволяет вытащить correlation_id из входящего батча, даже если в БД он пустой.
	query := `
		WITH inserted_rows AS (
			INSERT INTO sites (correlation_id, url, short_url, user_id)
			SELECT correlation_id, url, short_url, user_id FROM tmp_sites
			ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
			RETURNING url, short_url
		)
		SELECT t.correlation_id, i.short_url
		FROM inserted_rows i
		JOIN tmp_sites t ON i.url = t.url;
	`

	dbRows, err := tx.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("merge insert execution failed: %w", err)
	}
	defer dbRows.Close()

	actualAliases := make(map[string]model.ShortUrl)
	for dbRows.Next() {
		var correlationID string
		var shortURL model.ShortUrl
		if err := dbRows.Scan(&correlationID, &shortURL); err != nil {
			return nil, fmt.Errorf("scan merge returning row failed: %w", err)
		}
		// Мапим по гарантированно непустому ID текущего запроса
		actualAliases[correlationID] = shortURL
	}

	if err = dbRows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error during merge: %w", err)
	}

	return actualAliases, nil
}

// isShortUrlCollision проверяет, является ли ошибка нарушением уникальности индекса short_url.
func (r *PostgresSitesRepository) isShortUrlCollision(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// Код 23505 — unique_violation в PostgreSQL
		if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_url") {
			return true
		}
	}
	return false
}
