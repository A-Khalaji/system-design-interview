package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"system-design-interview/internal/model"
)

type ItemHandler struct {
	WriteDB *gorm.DB
	ReadDBs []*gorm.DB
	counter atomic.Uint64
}

func NewItemHandler(writeDB *gorm.DB, readDBs ...*gorm.DB) *ItemHandler {
	if len(readDBs) == 0 {
		readDBs = []*gorm.DB{writeDB}
	}

	return &ItemHandler{
		WriteDB: writeDB,
		ReadDBs: readDBs,
	}
}

func (h *ItemHandler) readDB() (*gorm.DB, string) {
	index := (h.counter.Add(1) - 1) % uint64(len(h.ReadDBs))

	name := "primary"
	if len(h.ReadDBs) > 1 {
		name = fmt.Sprintf("replica%d", index+1)
	}

	return h.ReadDBs[index], name
}

// CreateItem godoc
// @Summary Create an item
// @Tags items
// @Accept json
// @Produce json
// @Param item body model.Item true "Item data"
// @Success 201 {object} model.Item
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /items [post]
func (h *ItemHandler) CreateItem(c *gin.Context) {
	var item model.Item

	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item.ID = 0

	if err := h.WriteDB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create item"})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// ListItems godoc
// @Summary List items
// @Tags items
// @Produce json
// @Success 200 {array} model.Item
// @Failure 500 {object} map[string]string
// @Router /items [get]
func (h *ItemHandler) ListItems(c *gin.Context) {
	var items []model.Item

	db, replica := h.readDB()
	c.Header("X-Database-Replica", replica)

	if err := db.Order("id ASC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list items"})
		return
	}

	c.JSON(http.StatusOK, items)
}

// GetItem godoc
// @Summary Get an item
// @Tags items
// @Produce json
// @Param id path int true "Item ID"
// @Success 200 {object} model.Item
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /items/{id} [get]
func (h *ItemHandler) GetItem(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item ID"})
		return
	}

	db, replica := h.readDB()
	c.Header("X-Database-Replica", replica)

	var item model.Item
	err = db.First(&item, id).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get item"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// UpdateItem godoc
// @Summary Update an item
// @Tags items
// @Accept json
// @Produce json
// @Param id path int true "Item ID"
// @Param item body model.Item true "Updated item data"
// @Success 200 {object} model.Item
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /items/{id} [put]
func (h *ItemHandler) UpdateItem(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item ID"})
		return
	}

	var item model.Item

	if err := h.WriteDB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get item"})
		return
	}

	var input struct {
		Name        string  `json:"name" binding:"required"`
		Description string  `json:"description"`
		Price       float64 `json:"price" binding:"required,gte=0"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item.Name = input.Name
	item.Description = input.Description
	item.Price = input.Price

	if err := h.WriteDB.Save(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update item"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// DeleteItem godoc
// @Summary Delete an item
// @Tags items
// @Produce json
// @Param id path int true "Item ID"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /items/{id} [delete]
func (h *ItemHandler) DeleteItem(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item ID"})
		return
	}

	result := h.WriteDB.Delete(&model.Item{}, id)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete item"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}

	c.Status(http.StatusNoContent)
}
