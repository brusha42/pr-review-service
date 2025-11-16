package main

import (
	"database/sql"
	"log"

	"otbor_avito_november_2025/internal/api"
	"otbor_avito_november_2025/internal/handlers"
	"otbor_avito_november_2025/internal/service"
	"otbor_avito_november_2025/internal/store"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
)

func main() {
	dsn := "host=postgres user=postgres password=postgres dbname=otbor_avito port=5432 sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}
	store := store.NewPostgresStore(db)
	service := service.NewService(store)
	handler := handlers.NewHandler(service)
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	api.RegisterHandlers(e, handler)
	log.Println("Server starting on :8080")
	e.Logger.Fatal(e.Start(":8080"))
}
