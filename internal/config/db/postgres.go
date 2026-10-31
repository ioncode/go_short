package db

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/ioncode/go_short/migrations"
)

func RunPostgressMigrations(dbConnString string) error {
	d, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to initialize embed source driver: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, dbConnString)
	if err != nil {
		return fmt.Errorf("migration initialization failed: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed to execute: %w", err)
	}

	return nil
}
