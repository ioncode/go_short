package db

import (
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/ioncode/go_short/migrations"
)

func RunPostgressMigrations(dbConnString string) {
	// 2. Wrap the embedded filesystem using iofs wrapper
	d, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("Failed to initialize embed source driver: %v", err)
	}

	// 3. Pass the driver instance to golang-migrate
	m, err := migrate.NewWithSourceInstance("iofs", d, dbConnString)
	if err != nil {
		log.Fatalf("Migration initialization failed: %v", err)
	}

	// 4. Execute migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Migration failed to execute: %v", err)
	}

	log.Println("Migrations executed successfully from embedded FS!")
}
