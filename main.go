package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jatis/internal/api"
	"jatis/internal/config"
	"jatis/internal/database"
	"jatis/internal/messaging"
	"jatis/internal/services"

	"github.com/gin-gonic/gin"
)

// @title Multi-Tenant Messaging System API
// @version 1.0
// @description A multi-tenant messaging system with RabbitMQ and PostgreSQL
// @host localhost:8080
// @BasePath /api/v1
func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.NewConnection(cfg.Database.URL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize RabbitMQ
	rabbitmq, err := messaging.NewRabbitMQ(cfg.RabbitMQ.URL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer rabbitmq.Close()

	// Initialize services
	tenantManager := services.NewTenantManager(db, rabbitmq, cfg.Workers)
	messageService := services.NewMessageService(db)

	// Initialize HTTP server
	router := gin.Default()
	api.SetupRoutes(router, tenantManager, messageService)

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Println("Starting server on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown tenant manager first to stop all consumers
	tenantManager.Shutdown()

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}