package routes

import (
	"database/sql"
	"net/http"

	"auth-service/config"
	"auth-service/handlers"
	"auth-service/middleware"
	"auth-service/services"

	httpSwagger "github.com/swaggo/http-swagger"
)

func SetupRoutes(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessExpirationMinutes,
		cfg.JWTRefreshExpirationHours,
	)

	mux.HandleFunc("/auth/health", handlers.HealthHandler)
	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	createCredentialHandler := handlers.NewCreateCredentialHandler(db)
	mux.HandleFunc("/auth/internal/create-credential", createCredentialHandler.Handle)

	activateHandler := handlers.NewActivateHandler(db)
	mux.HandleFunc("/auth/activate", activateHandler.Handle)

	loginHandler := handlers.NewLoginHandler(db)
	mux.HandleFunc("/auth/login", loginHandler.Handle)

	refreshHandler := handlers.NewRefreshHandler(db)
	mux.HandleFunc("/auth/refresh", refreshHandler.Handle)

	protectedMeHandler := middleware.AuthMiddleware(jwtService)(http.HandlerFunc(handlers.MeHandler))
	mux.Handle("/auth/me", protectedMeHandler)

	forgotPasswordHandler := handlers.NewForgotPasswordHandler(db)
	mux.HandleFunc("/auth/forgot-password", forgotPasswordHandler.Handle)

	resetPasswordHandler := handlers.NewResetPasswordHandler(db)
	mux.HandleFunc("/auth/reset-password", resetPasswordHandler.Handle)

	return mux
}