package repository

import (
	"database/sql"

	"auth-service/models"
)

type CredentialRepository struct {
	DB *sql.DB
}

func NewCredentialRepository(db *sql.DB) *CredentialRepository {
	return &CredentialRepository{DB: db}
}

func (r *CredentialRepository) Create(credential *models.Credential) error {
	query := `
	INSERT INTO credentials (
		employee_id, email, password_hash, salt_password, is_active,
		activation_token, reset_token, reset_token_expires
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id, created_at, updated_at
	`

	return r.DB.QueryRow(
		query,
		credential.EmployeeID,
		credential.Email,
		credential.PasswordHash,
		credential.SaltPassword,
		credential.IsActive,
		credential.ActivationToken,
		credential.ResetToken,
		credential.ResetTokenExpires,
	).Scan(&credential.ID, &credential.CreatedAt, &credential.UpdatedAt)
}

func (r *CredentialRepository) GetByEmail(email string) (*models.Credential, error) {
	query := `
	SELECT id, employee_id, email, password_hash, salt_password, is_active,
	       activation_token, reset_token, reset_token_expires, created_at, updated_at
	FROM credentials
	WHERE email = $1
	`

	var credential models.Credential

	err := r.DB.QueryRow(query, email).Scan(
		&credential.ID,
		&credential.EmployeeID,
		&credential.Email,
		&credential.PasswordHash,
		&credential.SaltPassword,
		&credential.IsActive,
		&credential.ActivationToken,
		&credential.ResetToken,
		&credential.ResetTokenExpires,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &credential, nil
}

func (r *CredentialRepository) GetByActivationToken(token string) (*models.Credential, error) {
	query := `
	SELECT id, employee_id, email, password_hash, salt_password, is_active,
	       activation_token, reset_token, reset_token_expires, created_at, updated_at
	FROM credentials
	WHERE activation_token = $1
	`

	var credential models.Credential

	err := r.DB.QueryRow(query, token).Scan(
		&credential.ID,
		&credential.EmployeeID,
		&credential.Email,
		&credential.PasswordHash,
		&credential.SaltPassword,
		&credential.IsActive,
		&credential.ActivationToken,
		&credential.ResetToken,
		&credential.ResetTokenExpires,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &credential, nil
}

func (r *CredentialRepository) ExistsByEmail(email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM credentials WHERE email = $1)`

	var exists bool
	err := r.DB.QueryRow(query, email).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *CredentialRepository) ActivateAccount(id int64, passwordHash string, saltPassword string) error {
	query := `
	UPDATE credentials
	SET password_hash = $1,
	    salt_password = $2,
	    is_active = true,
	    activation_token = NULL,
	    updated_at = NOW()
	WHERE id = $3
	`

	_, err := r.DB.Exec(query, passwordHash, saltPassword, id)
	return err
}

func (r *CredentialRepository) GetByEmployeeID(employeeID int64) (*models.Credential, error) {
	query := `
	SELECT id, employee_id, email, password_hash, salt_password, is_active,
	       activation_token, reset_token, reset_token_expires, created_at, updated_at
	FROM credentials
	WHERE employee_id = $1
	`

	var credential models.Credential

	err := r.DB.QueryRow(query, employeeID).Scan(
		&credential.ID,
		&credential.EmployeeID,
		&credential.Email,
		&credential.PasswordHash,
		&credential.SaltPassword,
		&credential.IsActive,
		&credential.ActivationToken,
		&credential.ResetToken,
		&credential.ResetTokenExpires,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &credential, nil
}