package routes

import (
	"database/sql"
	"net/http"

	"auth-service/handlers"
)

func SetupRoutes(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/auth/health", handlers.HealthHandler)

	createCredentialHandler := handlers.NewCreateCredentialHandler(db)
	mux.HandleFunc("/auth/internal/create-credential", createCredentialHandler.Handle)

	return mux
}