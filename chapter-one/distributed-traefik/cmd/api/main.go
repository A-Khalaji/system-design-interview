package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	_ "system-design-interview/docs"
	"system-design-interview/internal/database"
	"system-design-interview/internal/model"
)

// @title System Design Interview API
// @version 1.0
// @description JSON API for practicing system design concepts.
// @BasePath /api/v1
func main() {
	gin.SetMode(gin.ReleaseMode)

	primaryDB, err := database.Connect(os.Getenv("DB_HOST"))
	if err != nil {
		log.Fatal(err)
	}

	replica1DB, err := database.Connect(os.Getenv("DB_REPLICA_HOST"))
	if err != nil {
		log.Fatal(err)
	}

	replica2DB, err := database.Connect(os.Getenv("DB_REPLICA_HOST_2"))
	if err != nil {
		log.Fatal(err)
	}

	if err := primaryDB.AutoMigrate(&model.Item{}); err != nil {
		log.Fatal(err)
	}

	router := SetupRouter(primaryDB, replica1DB, replica2DB)

	log.Println("API listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
