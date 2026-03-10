package routes

import (
	"database/sql"
	"net/http"
	"strings"

	"employee-service/config"
	"employee-service/handlers"
	"employee-service/middleware"
	"employee-service/repository"
	"employee-service/services"
	httpSwagger "github.com/swaggo/http-swagger"
)

func SetupRoutes(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(cfg.JWTSecret)

	employeeRepo := repository.NewEmployeeRepository(db)
	employeeService := services.NewEmployeeService(employeeRepo, nil)

	createEmployeeHandler := handlers.NewCreateEmployeeHandler(db)
	getEmployeesHandler := handlers.NewGetEmployeesHandler(db)
	getEmployeeByIDHandler := handlers.NewGetEmployeeByIDHandler(db)
	updateEmployeeHandler := handlers.NewUpdateEmployeeHandler(db)
	updateEmployeeActiveHandler := handlers.NewUpdateEmployeeActiveHandler(db)
	employeePermissionsHandler := handlers.NewEmployeePermissionsHandler(db)

	adminOnly := func(h http.Handler) http.Handler {
		return middleware.AuthMiddleware(jwtService)(
			middleware.AdminOnlyMiddleware(employeeService)(h),
		)
	}

	mux.HandleFunc("/employees/health", handlers.HealthHandler)
	mux.Handle("/swagger/", httpSwagger.WrapHandler)
	mux.Handle("/employees/admin/me", adminOnly(http.HandlerFunc(handlers.AdminMeHandler)))

	mux.Handle("/employees", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createEmployeeHandler.Handle(w, r)
		case http.MethodGet:
			getEmployeesHandler.Handle(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	mux.Handle("/employees/", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/employees/")
		if path == "" {
			http.Error(w, "employee id is required", http.StatusBadRequest)
			return
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/active"):
			if r.Method != http.MethodPatch {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			updateEmployeeActiveHandler.Handle(w, r)

		case strings.HasSuffix(r.URL.Path, "/permissions"):
			employeePermissionsHandler.Handle(w, r)

		default:
			switch r.Method {
			case http.MethodGet:
				getEmployeeByIDHandler.Handle(w, r)
			case http.MethodPut:
				updateEmployeeHandler.Handle(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}
	})))

	return mux
}