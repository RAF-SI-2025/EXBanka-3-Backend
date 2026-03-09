package database

import (
	"database/sql"
	"log"
)

func RunMigrations(db *sql.DB) error {
	dropUsersTable := `DROP TABLE IF EXISTS users;`
	_, err := db.Exec(dropUsersTable)
	if err != nil {
		return err
	}

	createCredentialsTable := `
	CREATE TABLE IF NOT EXISTS credentials (
		id SERIAL PRIMARY KEY,
		employee_id BIGINT UNIQUE NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash TEXT,
		salt_password TEXT,
		is_active BOOLEAN NOT NULL DEFAULT false,
		activation_token TEXT,
		reset_token TEXT,
		reset_token_expires TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);
	`

	_, err = db.Exec(createCredentialsTable)
	if err != nil {
		return err
	}

	log.Println("database migrations executed successfully")
	return nil
}