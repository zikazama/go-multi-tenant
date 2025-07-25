package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"jatis/internal/database"
	"jatis/internal/messaging"
	"jatis/internal/metrics"
	"jatis/internal/models"

	"github.com/google/uuid"
)

type TenantManager struct {
	db           *sql.DB
	rabbitmq     *messaging.RabbitMQ
	consumers    map[string]*messaging.Consumer
	workerPools  map[string]*WorkerPool
	mu           sync.RWMutex
	defaultWorkers int
}

type WorkerPool struct {
	workers   int32
	jobQueue  chan []byte
	quit      chan bool
	wg        sync.WaitGroup
}

func NewTenantManager(db *sql.DB, rabbitmq *messaging.RabbitMQ, defaultWorkers int) *TenantManager {
	tm := &TenantManager{
		db:             db,
		rabbitmq:       rabbitmq,
		consumers:      make(map[string]*messaging.Consumer),
		workerPools:    make(map[string]*WorkerPool),
		defaultWorkers: defaultWorkers,
	}

	// Load existing tenants and start their consumers
	tm.loadExistingTenants()

	return tm
}

func (tm *TenantManager) CreateTenant(name string) (*models.Tenant, error) {
	tenantID := uuid.New().String()

	// Create tenant in database
	query := `INSERT INTO tenants (id, name) VALUES ($1, $2) RETURNING created_at, updated_at`
	var tenant models.Tenant
	tenant.ID = tenantID
	tenant.Name = name

	err := tm.db.QueryRow(query, tenantID, name).Scan(&tenant.CreatedAt, &tenant.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// Create partition for tenant
	if err := database.CreateTenantPartition(tm.db, tenantID); err != nil {
		return nil, fmt.Errorf("failed to create tenant partition: %w", err)
	}

	// Create tenant config
	configQuery := `INSERT INTO tenant_configs (tenant_id, workers) VALUES ($1, $2)`
	if _, err := tm.db.Exec(configQuery, tenantID, tm.defaultWorkers); err != nil {
		return nil, fmt.Errorf("failed to create tenant config: %w", err)
	}

	// Start consumer for tenant
	if err := tm.startTenantConsumer(tenantID); err != nil {
		return nil, fmt.Errorf("failed to start tenant consumer: %w", err)
	}

	// Update metrics
	metrics.IncrementActiveTenants()

	return &tenant, nil
}

func (tm *TenantManager) DeleteTenant(tenantID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Stop consumer
	if consumer, exists := tm.consumers[tenantID]; exists {
		consumer.Stop()
		delete(tm.consumers, tenantID)
	}

	// Stop worker pool
	if pool, exists := tm.workerPools[tenantID]; exists {
		pool.Stop()
		delete(tm.workerPools, tenantID)
	}

	// Delete RabbitMQ queue
	if err := tm.rabbitmq.DeleteTenantQueue(tenantID); err != nil {
		log.Printf("Warning: failed to delete RabbitMQ queue: %v", err)
	}

	// Delete from database (cascade will handle configs and messages)
	query := `DELETE FROM tenants WHERE id = $1`
	if _, err := tm.db.Exec(query, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	// Drop partition
	if err := database.DropTenantPartition(tm.db, tenantID); err != nil {
		log.Printf("Warning: failed to drop partition: %v", err)
	}

	// Update metrics
	metrics.DecrementActiveTenants()

	return nil
}

func (tm *TenantManager) GetTenant(tenantID string) (*models.Tenant, error) {
	query := `SELECT id, name, created_at, updated_at FROM tenants WHERE id = $1`
	var tenant models.Tenant

	err := tm.db.QueryRow(query, tenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &tenant, nil
}

func (tm *TenantManager) ListTenants() ([]*models.Tenant, error) {
	query := `SELECT id, name, created_at, updated_at FROM tenants ORDER BY created_at DESC`
	rows, err := tm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*models.Tenant
	for rows.Next() {
		var tenant models.Tenant
		err := rows.Scan(&tenant.ID, &tenant.Name, &tenant.CreatedAt, &tenant.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &tenant)
	}

	return tenants, nil
}

func (tm *TenantManager) UpdateConcurrency(tenantID string, workers int) error {
	// Update database
	query := `UPDATE tenant_configs SET workers = $1, updated_at = NOW() WHERE tenant_id = $2`
	result, err := tm.db.Exec(query, workers, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update concurrency: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}

	// Update worker pool
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if pool, exists := tm.workerPools[tenantID]; exists {
		pool.UpdateWorkers(int32(workers))
	}

	return nil
}

func (tm *TenantManager) startTenantConsumer(tenantID string) error {
	consumer, err := tm.rabbitmq.CreateTenantQueue(tenantID)
	if err != nil {
		return err
	}

	// Get worker count for tenant
	var workers int
	query := `SELECT workers FROM tenant_configs WHERE tenant_id = $1`
	err = tm.db.QueryRow(query, tenantID).Scan(&workers)
	if err != nil {
		workers = tm.defaultWorkers
	}

	// Create worker pool
	pool := NewWorkerPool(int32(workers))
	
	tm.mu.Lock()
	tm.consumers[tenantID] = consumer
	tm.workerPools[tenantID] = pool
	tm.mu.Unlock()

	// Start consumer with message handler
	consumer.Start(func(body []byte) error {
		return tm.processMessage(tenantID, body, pool)
	})

	return nil
}

func (tm *TenantManager) processMessage(tenantID string, body []byte, pool *WorkerPool) error {
	// Send message to worker pool for processing
	select {
	case pool.jobQueue <- body:
		return nil
	default:
		return fmt.Errorf("worker pool queue is full")
	}
}

func (tm *TenantManager) loadExistingTenants() {
	tenants, err := tm.ListTenants()
	if err != nil {
		log.Printf("Failed to load existing tenants: %v", err)
		return
	}

	for _, tenant := range tenants {
		if err := tm.startTenantConsumer(tenant.ID); err != nil {
			log.Printf("Failed to start consumer for tenant %s: %v", tenant.ID, err)
		}
	}
}

func (tm *TenantManager) Shutdown() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Stop all consumers
	for _, consumer := range tm.consumers {
		consumer.Stop()
	}

	// Stop all worker pools
	for _, pool := range tm.workerPools {
		pool.Stop()
	}

	log.Println("All tenant consumers and worker pools stopped")
}

// WorkerPool implementation
func NewWorkerPool(workers int32) *WorkerPool {
	pool := &WorkerPool{
		workers:  workers,
		jobQueue: make(chan []byte, 100), // Buffered channel
		quit:     make(chan bool),
	}

	pool.start()
	return pool
}

func (wp *WorkerPool) start() {
	for i := int32(0); i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	
	for {
		select {
		case job := <-wp.jobQueue:
			wp.processJob(job)
		case <-wp.quit:
			return
		}
	}
}

func (wp *WorkerPool) processJob(body []byte) {
	// Process the message (placeholder implementation)
	var message map[string]interface{}
	if err := json.Unmarshal(body, &message); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return
	}

	log.Printf("Processing message: %v", message)
	// Add actual message processing logic here
}

func (wp *WorkerPool) UpdateWorkers(newWorkers int32) {
	currentWorkers := atomic.LoadInt32(&wp.workers)
	
	if newWorkers > currentWorkers {
		// Add workers
		for i := currentWorkers; i < newWorkers; i++ {
			wp.wg.Add(1)
			go wp.worker()
		}
	} else if newWorkers < currentWorkers {
		// Remove workers by sending quit signals
		for i := newWorkers; i < currentWorkers; i++ {
			wp.quit <- true
		}
	}

	atomic.StoreInt32(&wp.workers, newWorkers)
}

func (wp *WorkerPool) Stop() {
	close(wp.quit)
	wp.wg.Wait()
}