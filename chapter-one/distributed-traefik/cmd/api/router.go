package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"system-design-interview/internal/handler"
)

func SetupRouter(
	primaryDB *gorm.DB,
	replica1DB *gorm.DB,
	replica2DB *gorm.DB,
) *gin.Engine {
	router := gin.Default()

	router.Use(func(c *gin.Context) {
		instance := os.Getenv("API_INSTANCE")
		if instance == "" {
			instance = "unknown"
		}

		c.Header("X-Api-Instance", instance)
		c.Next()
	})

	itemHandler := handler.NewItemHandler(
		primaryDB,
		[]*gorm.DB{replica1DB, replica2DB},
	)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := router.Group("/api/v1")
	{
		api.POST("/items", itemHandler.CreateItem)
		api.GET("/items", itemHandler.ListItems)
		api.GET("/items/:id", itemHandler.GetItem)
		api.PUT("/items/:id", itemHandler.UpdateItem)
		api.DELETE("/items/:id", itemHandler.DeleteItem)
	}

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return router
}
