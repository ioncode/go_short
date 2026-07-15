package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/ioncode/go_short/internal/model"
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
		"SELECT url "+
			"FROM sites WHERE short_url = $1 LIMIT 1", alias)

	var site model.Site
	err := row.Scan(&site.Url)
	if err != nil {
		if err == sql.ErrNoRows {
			return site, ErrSiteNotFound
		}
		return site, err
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
		if err == sql.ErrNoRows {
			return site, ErrSiteNotFound
		}
		return site, err
	}

	site.Url = url
	return site, nil
}

func (r *PostgresSitesRepository) StoreSite(site model.Site) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(ctx, "INSERT INTO SITES (url, short_url) VALUES ($1, $2)", site.Url, site.ShortUrl)
	if err != nil {
		return err
	}

	return nil
}

func (r *PostgresSitesRepository) Close() error {
	return r.db.Close()
}
