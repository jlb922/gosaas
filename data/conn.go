package data

import (
	"database/sql"

	"github.com/jlb922/gosaas/data/postgres"
	_ "github.com/lib/pq"
)

// Open creates the database connection and initialize the postgres services.
func (db *DB) Open(driverName, dataSource string) error {
	conn, err := sql.Open(driverName, dataSource)
	if err != nil {
		return err
	}

	if err := conn.Ping(); err != nil {
		return err
	}

	db.Users = &postgres.Users{DB: conn}
	db.Webhooks = &postgres.Webhooks{DB: conn}

	db.Connection = conn

	db.DatabaseName = "gosaas"
	return nil
}

func (db *DB) Close() {
	db.Connection.Close()
}
