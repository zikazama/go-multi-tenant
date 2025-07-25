package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"jatis/internal/models"

	"github.com/google/uuid"
)

type MessageService struct {
	db *sql.DB
}

type PaginatedMessages struct {
	Data       []*models.Message `json:"data"`
	NextCursor *string           `json:"next_cursor"`
}

func NewMessageService(db *sql.DB) *MessageService {
	return &MessageService{db: db}
}

func (ms *MessageService) CreateMessage(tenantID string, payload interface{}) (*models.Message, error) {
	messageID := uuid.New().String()
	
	// Convert payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	query := `
		INSERT INTO messages (id, tenant_id, payload) 
		VALUES ($1, $2, $3) 
		RETURNING created_at
	`
	
	var message models.Message
	message.ID = messageID
	message.TenantID = tenantID
	message.Payload = payload

	err = ms.db.QueryRow(query, messageID, tenantID, payloadBytes).Scan(&message.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	return &message, nil
}

func (ms *MessageService) GetMessages(tenantID string, cursor *string, limit int) (*PaginatedMessages, error) {
	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}

	var query string
	var args []interface{}

	if cursor != nil && *cursor != "" {
		// Parse cursor (timestamp)
		cursorTime, err := time.Parse(time.RFC3339, *cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor format: %w", err)
		}

		query = `
			SELECT id, tenant_id, payload, created_at 
			FROM messages 
			WHERE tenant_id = $1 AND created_at < $2 
			ORDER BY created_at DESC 
			LIMIT $3
		`
		args = []interface{}{tenantID, cursorTime, limit + 1} // +1 to check if there's a next page
	} else {
		query = `
			SELECT id, tenant_id, payload, created_at 
			FROM messages 
			WHERE tenant_id = $1 
			ORDER BY created_at DESC 
			LIMIT $2
		`
		args = []interface{}{tenantID, limit + 1}
	}

	rows, err := ms.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*models.Message
	for rows.Next() {
		var message models.Message
		var payloadBytes []byte
		err := rows.Scan(
			&message.ID,
			&message.TenantID,
			&payloadBytes,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		
		// Unmarshal payload
		var payload interface{}
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		message.Payload = payload
		
		messages = append(messages, &message)
	}

	result := &PaginatedMessages{
		Data: messages,
	}

	// Check if there are more messages (next page)
	if len(messages) > limit {
		// Remove the extra message
		result.Data = messages[:limit]
		// Set next cursor to the last message's timestamp
		lastMessage := messages[limit-1]
		nextCursor := lastMessage.CreatedAt.Format(time.RFC3339)
		result.NextCursor = &nextCursor
	}

	return result, nil
}

func (ms *MessageService) GetMessage(messageID string) (*models.Message, error) {
	query := `
		SELECT id, tenant_id, payload, created_at 
		FROM messages 
		WHERE id = $1
	`

	var message models.Message
	var payloadBytes []byte
	err := ms.db.QueryRow(query, messageID).Scan(
		&message.ID,
		&message.TenantID,
		&payloadBytes,
		&message.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message not found")
		}
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	// Unmarshal payload
	var payload interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	message.Payload = payload

	return &message, nil
}

func (ms *MessageService) GetMessagesByTenant(tenantID string) ([]*models.Message, error) {
	query := `
		SELECT id, tenant_id, payload, created_at 
		FROM messages 
		WHERE tenant_id = $1 
		ORDER BY created_at DESC
	`

	rows, err := ms.db.Query(query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*models.Message
	for rows.Next() {
		var message models.Message
		var payloadBytes []byte
		err := rows.Scan(
			&message.ID,
			&message.TenantID,
			&payloadBytes,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		
		// Unmarshal payload
		var payload interface{}
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		message.Payload = payload
		
		messages = append(messages, &message)
	}

	return messages, nil
}

func (ms *MessageService) DeleteMessage(messageID string) error {
	query := `DELETE FROM messages WHERE id = $1`
	result, err := ms.db.Exec(query, messageID)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found")
	}

	return nil
}

func (ms *MessageService) GetMessageStats(tenantID string) (*models.MessageStats, error) {
	query := `
		SELECT 
			COUNT(*) as total_messages,
			COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours') as messages_24h,
			COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '1 hour') as messages_1h
		FROM messages 
		WHERE tenant_id = $1
	`

	var stats models.MessageStats
	err := ms.db.QueryRow(query, tenantID).Scan(
		&stats.TotalMessages,
		&stats.Messages24h,
		&stats.Messages1h,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get message stats: %w", err)
	}

	return &stats, nil
}