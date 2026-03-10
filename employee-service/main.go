package main

import (
	"log"
	"net/http"

	"employee-service/config"
	"employee-service/database"
	"employee-service/routes"
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
	
	log.Printf("employee-service started on :%s\n", cfg.AppPort)
	err = http.ListenAndServe(":"+cfg.AppPort, router)
	if err != nil {
		log.Fatal("failed to start server: ", err)
	}
}