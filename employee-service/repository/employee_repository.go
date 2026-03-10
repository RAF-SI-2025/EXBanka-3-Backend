package repository

import (
	"database/sql"
	"strconv"

	"employee-service/models"
)

type EmployeeRepository struct {
	DB *sql.DB
}

func NewEmployeeRepository(db *sql.DB) *EmployeeRepository {
	return &EmployeeRepository{DB: db}
}

func (r *EmployeeRepository) Create(employee *models.Employee) error {

	query := `
	INSERT INTO employees (
		first_name,
		last_name,
		date_of_birth,
		gender,
		email,
		phone_number,
		address,
		username,
		position,
		department,
		active
	)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true)
	RETURNING id, created_at, updated_at
	`

	return r.DB.QueryRow(
		query,
		employee.FirstName,
		employee.LastName,
		employee.DateOfBirth,
		employee.Gender,
		employee.Email,
		employee.PhoneNumber,
		employee.Address,
		employee.Username,
		employee.Position,
		employee.Department,
	).Scan(&employee.ID, &employee.CreatedAt, &employee.UpdatedAt)
}

func (r *EmployeeRepository) GetAll(filter *models.EmployeeFilter) ([]models.Employee, error) {
	query := `
	SELECT id, first_name, last_name, date_of_birth, gender, email,
	       phone_number, address, username, position, department,
	       active, created_at, updated_at
	FROM employees
	WHERE 1=1
	`

	args := []any{}
	argPos := 1

	if filter.Email != "" {
		query += " AND email ILIKE $" + strconv.Itoa(argPos)
		args = append(args, "%"+filter.Email+"%")
		argPos++
	}

	if filter.FirstName != "" {
		query += " AND first_name ILIKE $" + strconv.Itoa(argPos)
		args = append(args, "%"+filter.FirstName+"%")
		argPos++
	}

	if filter.LastName != "" {
		query += " AND last_name ILIKE $" + strconv.Itoa(argPos)
		args = append(args, "%"+filter.LastName+"%")
		argPos++
	}

	if filter.Position != "" {
		query += " AND position ILIKE $" + strconv.Itoa(argPos)
		args = append(args, "%"+filter.Position+"%")
		argPos++
	}

	query += " ORDER BY id ASC"

	rows, err := r.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var employees []models.Employee

	for rows.Next() {
		var employee models.Employee

		err := rows.Scan(
			&employee.ID,
			&employee.FirstName,
			&employee.LastName,
			&employee.DateOfBirth,
			&employee.Gender,
			&employee.Email,
			&employee.PhoneNumber,
			&employee.Address,
			&employee.Username,
			&employee.Position,
			&employee.Department,
			&employee.Active,
			&employee.CreatedAt,
			&employee.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		employees = append(employees, employee)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return employees, nil
}