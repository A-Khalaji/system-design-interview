package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"system-design-interview/internal/handler"
)

func SetupRouter(db *gorm.DB) *gin.Engine {
	router := gin.Default()

	itemHandler := handler.NewItemHandler(db)

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
