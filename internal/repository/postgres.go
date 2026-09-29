package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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

func (r *PostgresSitesRepository) GetByUser(userId string) ([]model.UserSitesResponseItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	records := []model.UserSitesResponseItem{}

	query := `SELECT url, short_url FROM sites WHERE user_id = $1`
	rows, err := r.db.QueryContext(ctx, query, userId)
	if err != nil {
		return records, err
	}
	defer rows.Close()

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

func (r *PostgresSitesRepository) BatchStoreSites(sites []model.Site) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	connection, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	rows := [][]any{}

	for _, site := range sites {
		rows = append(rows, []any{site.Url, site.ShortUrl, site.CorrelationId, site.UserId})
	}

	err = connection.Raw(func(driverConn any) error {
		stdlibConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return errors.New("driver connection is not a pgx stdlib connection")
		}

		pgxConn := stdlibConn.Conn()
		_, err = pgxConn.CopyFrom(
			ctx,
			pgx.Identifier{"sites"},
			[]string{"url", "short_url", "correlation_id", "user_id"},
			pgx.CopyFromRows(rows),
		)
		return err
	})

	return err
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
