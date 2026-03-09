package main

import (
	"log"
	"net/http"

	"auth-service/config"
	"auth-service/database"
	"auth-service/routes"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.ConnectPostgres(cfg)
	if err != nil {
		log.Fatal("failed to connect to database: ", err)
	}
	defer db.Close()

	err = database.RunMigrations(db)
	if err != nil {
		log.Fatal("failed to run migrations: ", err)
	}

	router := routes.SetupRoutes(db)


	log.Printf("auth-service started on :%s\n", cfg.AppPort)
	err = http.ListenAndServe(":"+cfg.AppPort, router)
	if err != nil {
		log.Fatal("failed to start server: ", err)
	}
}