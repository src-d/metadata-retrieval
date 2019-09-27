package database

import (
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Migrate updates the DB schema to the latest version
func Migrate(databaseURL string) error {
	m, err := migrate.New(
		"file://database/migrations",
		databaseURL)
	if err != nil {
		return err
	}
	return m.Up()
}
