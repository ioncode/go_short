package repository

import (
	"context"
	"database/sql"

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
	return model.Site{
		Url: "ya.ru",
	}, nil
}

func (r *PostgresSitesRepository) GetByUrl(url model.Url) (model.Site, error) {
	return model.Site{
		Url: "ya.ru",
	}, nil
}

func (r *PostgresSitesRepository) StoreSite(site model.Site) error {
	return nil
}

func (r *PostgresSitesRepository) Close() error {
	return r.db.Close()
}
