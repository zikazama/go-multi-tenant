package models

import (
	"time"
)

type Tenant struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Message struct {
	ID        string      `json:"id" db:"id"`
	TenantID  string      `json:"tenant_id" db:"tenant_id"`
	Payload   interface{} `json:"payload" db:"payload" swaggertype:"object"`
	CreatedAt time.Time   `json:"created_at" db:"created_at"`
}

type TenantConfig struct {
	TenantID  string    `json:"tenant_id" db:"tenant_id"`
	Workers   int       `json:"workers" db:"workers"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type MessageStats struct {
	TotalMessages int64 `json:"total_messages"`
	Messages24h   int64 `json:"messages_24h"`
	Messages1h    int64 `json:"messages_1h"`
}

// Request/Response DTOs
type CreateTenantRequest struct {
	Name string `json:"name" binding:"required"`
}

type CreateMessageRequest struct {
	Payload interface{} `json:"payload" binding:"required" swaggertype:"object"`
}

type UpdateConcurrencyRequest struct {
	Workers int `json:"workers" binding:"required,min=1,max=100"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}