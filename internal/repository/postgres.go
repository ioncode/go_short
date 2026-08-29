package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ioncode/go_short/internal/model"
	"github.com/jackc/pgerrcode"
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

func (r *PostgresSitesRepository) StoreSite(site model.Site) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(ctx, "INSERT INTO SITES (url, short_url, user_id) VALUES ($1, $2, $3)", site.Url, site.ShortUrl, site.UserId)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return ErrSiteExists
			}
		}
		return err
	}

	return nil
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
