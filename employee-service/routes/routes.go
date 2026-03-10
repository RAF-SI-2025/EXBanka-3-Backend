package routes

import (
	"database/sql"
	"net/http"

	"employee-service/handlers"
)

func SetupRoutes(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/employees/health", handlers.HealthHandler)

	createEmployeeHandler := handlers.NewCreateEmployeeHandler(db)
	getEmployeesHandler := handlers.NewGetEmployeesHandler(db)

	mux.HandleFunc("/employees", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createEmployeeHandler.Handle(w, r)
		case http.MethodGet:
			getEmployeesHandler.Handle(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return mux
}