package repository

import (
	"context"
	"database/sql"

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
