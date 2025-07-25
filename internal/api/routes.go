package api

import (
	"net/http"
	"strconv"

	"jatis/internal/metrics"
	"jatis/internal/models"
	"jatis/internal/services"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRoutes(router *gin.Engine, tenantManager *services.TenantManager, messageService *services.MessageService) {
	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(metrics.PrometheusMiddleware())

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Metrics endpoint
	router.GET("/metrics", metrics.MetricsHandler())

	// API routes
	api := router.Group("/api/v1")
	{
		// Tenant routes
		tenants := api.Group("/tenants")
		{
			tenants.POST("", createTenant(tenantManager))
			tenants.GET("", listTenants(tenantManager))
			tenants.GET("/:id", getTenant(tenantManager))
			tenants.DELETE("/:id", deleteTenant(tenantManager))
			tenants.PUT("/:id/config/concurrency", updateConcurrency(tenantManager))
		}

		// Message routes
		messages := api.Group("/messages")
		{
			messages.GET("", getMessages(messageService))
			messages.POST("/:tenant_id", createMessage(messageService))
			messages.GET("/:id", getMessage(messageService))
			messages.DELETE("/:id", deleteMessage(messageService))
		}

		// Stats routes
		stats := api.Group("/stats")
		{
			stats.GET("/tenants/:id/messages", getMessageStats(messageService))
		}
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
}

// @Summary Create a new tenant
// @Description Create a new tenant with automatic consumer setup
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenant body models.CreateTenantRequest true "Tenant data"
// @Success 201 {object} models.Tenant
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /tenants [post]
func createTenant(tm *services.TenantManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.CreateTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "Invalid request",
				Message: err.Error(),
			})
			return
		}

		tenant, err := tm.CreateTenant(req.Name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to create tenant",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, tenant)
	}
}

// @Summary List all tenants
// @Description Get a list of all tenants
// @Tags tenants
// @Produce json
// @Success 200 {array} models.Tenant
// @Failure 500 {object} models.ErrorResponse
// @Router /tenants [get]
func listTenants(tm *services.TenantManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenants, err := tm.ListTenants()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to list tenants",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, tenants)
	}
}

// @Summary Get a tenant by ID
// @Description Get a specific tenant by its ID
// @Tags tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} models.Tenant
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /tenants/{id} [get]
func getTenant(tm *services.TenantManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		tenant, err := tm.GetTenant(tenantID)
		if err != nil {
			if err.Error() == "tenant not found" {
				c.JSON(http.StatusNotFound, models.ErrorResponse{
					Error: "Tenant not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to get tenant",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, tenant)
	}
}

// @Summary Delete a tenant
// @Description Delete a tenant and stop its consumer
// @Tags tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /tenants/{id} [delete]
func deleteTenant(tm *services.TenantManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		err := tm.DeleteTenant(tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to delete tenant",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, models.SuccessResponse{
			Message: "Tenant deleted successfully",
		})
	}
}

// @Summary Update tenant concurrency
// @Description Update the number of workers for a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID"
// @Param config body models.UpdateConcurrencyRequest true "Concurrency config"
// @Success 200 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /tenants/{id}/config/concurrency [put]
func updateConcurrency(tm *services.TenantManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		var req models.UpdateConcurrencyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "Invalid request",
				Message: err.Error(),
			})
			return
		}

		err := tm.UpdateConcurrency(tenantID, req.Workers)
		if err != nil {
			if err.Error() == "tenant not found" {
				c.JSON(http.StatusNotFound, models.ErrorResponse{
					Error: "Tenant not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to update concurrency",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, models.SuccessResponse{
			Message: "Concurrency updated successfully",
		})
	}
}

// @Summary Get messages with pagination
// @Description Get messages with cursor-based pagination
// @Tags messages
// @Produce json
// @Param tenant_id query string true "Tenant ID"
// @Param cursor query string false "Cursor for pagination"
// @Param limit query int false "Limit (default 20, max 100)"
// @Success 200 {object} services.PaginatedMessages
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages [get]
func getMessages(ms *services.MessageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Query("tenant_id")
		if tenantID == "" {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error: "tenant_id query parameter is required",
			})
			return
		}

		cursor := c.Query("cursor")
		var cursorPtr *string
		if cursor != "" {
			cursorPtr = &cursor
		}

		limit := 20 // default
		if limitStr := c.Query("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil {
				limit = l
			}
		}

		messages, err := ms.GetMessages(tenantID, cursorPtr, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to get messages",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, messages)
	}
}

// @Summary Create a message
// @Description Create a new message for a tenant
// @Tags messages
// @Accept json
// @Produce json
// @Param tenant_id path string true "Tenant ID"
// @Param message body models.CreateMessageRequest true "Message data"
// @Success 201 {object} models.Message
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{tenant_id} [post]
func createMessage(ms *services.MessageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("tenant_id")

		var req models.CreateMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "Invalid request",
				Message: err.Error(),
			})
			return
		}

		message, err := ms.CreateMessage(tenantID, req.Payload)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to create message",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, message)
	}
}

// @Summary Get a message by ID
// @Description Get a specific message by its ID
// @Tags messages
// @Produce json
// @Param id path string true "Message ID"
// @Success 200 {object} models.Message
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{id} [get]
func getMessage(ms *services.MessageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		messageID := c.Param("id")

		message, err := ms.GetMessage(messageID)
		if err != nil {
			if err.Error() == "message not found" {
				c.JSON(http.StatusNotFound, models.ErrorResponse{
					Error: "Message not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to get message",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, message)
	}
}

// @Summary Delete a message
// @Description Delete a message by its ID
// @Tags messages
// @Produce json
// @Param id path string true "Message ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /messages/{id} [delete]
func deleteMessage(ms *services.MessageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		messageID := c.Param("id")

		err := ms.DeleteMessage(messageID)
		if err != nil {
			if err.Error() == "message not found" {
				c.JSON(http.StatusNotFound, models.ErrorResponse{
					Error: "Message not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to delete message",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, models.SuccessResponse{
			Message: "Message deleted successfully",
		})
	}
}

// @Summary Get message statistics
// @Description Get message statistics for a tenant
// @Tags stats
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} models.MessageStats
// @Failure 500 {object} models.ErrorResponse
// @Router /stats/tenants/{id}/messages [get]
func getMessageStats(ms *services.MessageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		stats, err := ms.GetMessageStats(tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "Failed to get message stats",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}