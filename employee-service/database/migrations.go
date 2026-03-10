package database

import (
	"database/sql"
	"log"
)

func RunMigrations(db *sql.DB) error {
	createEmployeesTable := `
	CREATE TABLE IF NOT EXISTS employees (
		id SERIAL PRIMARY KEY,
		first_name VARCHAR(100) NOT NULL,
		last_name VARCHAR(100) NOT NULL,
		date_of_birth DATE NOT NULL,
		gender VARCHAR(20) NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		phone_number VARCHAR(50) NOT NULL,
		address TEXT NOT NULL,
		username VARCHAR(100) UNIQUE NOT NULL,
		position VARCHAR(100) NOT NULL,
		department VARCHAR(100) NOT NULL,
		active BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);
	`

	_, err := db.Exec(createEmployeesTable)
	if err != nil {
		return err
	}

	createEmployeePermissionsTable := `
	CREATE TABLE IF NOT EXISTS employee_permissions (
		id SERIAL PRIMARY KEY,
		employee_id BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
		permission VARCHAR(100) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		UNIQUE(employee_id, permission)
	);
	`

	_, err = db.Exec(createEmployeePermissionsTable)
	if err != nil {
		return err
	}

	log.Println("employee-service migrations executed successfully")
	return nil
}