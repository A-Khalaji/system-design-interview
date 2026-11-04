package main

import (
	"log"

	"github.com/gin-gonic/gin"

	_ "system-design-interview/docs"
	"system-design-interview/internal/database"
	"system-design-interview/internal/model"
)

func main() {
	gin.SetMode(gin.ReleaseMode)

	writeDB, err := database.Connect("postgres", "5432")
	if err != nil {
		log.Fatal(err)
	}

	replica1, err := database.Connect("replica1", "5432")
	if err != nil {
		log.Fatal(err)
	}

	replica2, err := database.Connect("replica2", "5432")
	if err != nil {
		log.Fatal(err)
	}

	writeSQLDB, err := writeDB.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer writeSQLDB.Close()

	replica1SQLDB, err := replica1.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer replica1SQLDB.Close()

	replica2SQLDB, err := replica2.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer replica2SQLDB.Close()

	if err := writeDB.AutoMigrate(&model.Item{}); err != nil {
		log.Fatal(err)
	}

	router := SetupRouter(writeDB, replica1, replica2)

	log.Println("API listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
