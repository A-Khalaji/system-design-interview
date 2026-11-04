package main

import (
	"log"

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

	db, err := database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	if err := db.AutoMigrate(&model.Item{}); err != nil {
		log.Fatal(err)
	}

	router := SetupRouter(db)

	log.Println("API listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
