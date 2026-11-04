package model

import "time"

type Item struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:150;not null" json:"name" binding:"required"`
	Description string    `gorm:"type:text" json:"description"`
	Price       float64   `gorm:"type:numeric(12,2);not null" json:"price" binding:"required,gte=0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
