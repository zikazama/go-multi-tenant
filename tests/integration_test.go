package tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jatis/internal/api"
	"jatis/internal/database"
	"jatis/internal/messaging"
	"jatis/internal/models"
	"jatis/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	suite.Suite
	pool           *dockertest.Pool
	postgresRes    *dockertest.Resource
	rabbitmqRes    *dockertest.Resource
	db             *sql.DB
	rabbitmq       *messaging.RabbitMQ
	router         *gin.Engine
	tenantManager  *services.TenantManager
	messageService *services.MessageService
}

func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (suite *IntegrationTestSuite) SetupSuite() {
	var err error

	// Create Docker pool
	suite.pool, err = dockertest.NewPool("")
	suite.Require().NoError(err)

	// Start PostgreSQL container
	suite.postgresRes, err = suite.pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "13",
		Env: []string{
			"POSTGRES_PASSWORD=testpass",
			"POSTGRES_DB=testdb",
			"POSTGRES_USER=testuser",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	suite.Require().NoError(err)

	// Start RabbitMQ container
	suite.rabbitmqRes, err = suite.pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "rabbitmq",
		Tag:        "3-management",
		Env: []string{
			"RABBITMQ_DEFAULT_USER=testuser",
			"RABBITMQ_DEFAULT_PASS=testpass",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	suite.Require().NoError(err)

	// Wait for PostgreSQL to be ready
	postgresURL := fmt.Sprintf("postgres://testuser:testpass@localhost:%s/testdb?sslmode=disable",
		suite.postgresRes.GetPort("5432/tcp"))

	suite.pool.MaxWait = 120 * time.Second
	err = suite.pool.Retry(func() error {
		suite.db, err = database.NewConnection(postgresURL)
		if err != nil {
			return err
		}
		return suite.db.Ping()
	})
	suite.Require().NoError(err)

	// Wait for RabbitMQ to be ready
	rabbitmqURL := fmt.Sprintf("amqp://testuser:testpass@localhost:%s/",
		suite.rabbitmqRes.GetPort("5672/tcp"))

	err = suite.pool.Retry(func() error {
		suite.rabbitmq, err = messaging.NewRabbitMQ(rabbitmqURL)
		return err
	})
	suite.Require().NoError(err)

	// Run migrations
	err = database.RunMigrations(suite.db)
	suite.Require().NoError(err)

	// Initialize services
	suite.tenantManager = services.NewTenantManager(suite.db, suite.rabbitmq, 3)
	suite.messageService = services.NewMessageService(suite.db)

	// Setup router
	gin.SetMode(gin.TestMode)
	suite.router = gin.New()
	api.SetupRoutes(suite.router, suite.tenantManager, suite.messageService)
}

func (suite *IntegrationTestSuite) TearDownSuite() {
	if suite.tenantManager != nil {
		suite.tenantManager.Shutdown()
	}
	if suite.db != nil {
		suite.db.Close()
	}
	if suite.rabbitmq != nil {
		suite.rabbitmq.Close()
	}
	if suite.postgresRes != nil {
		suite.pool.Purge(suite.postgresRes)
	}
	if suite.rabbitmqRes != nil {
		suite.pool.Purge(suite.rabbitmqRes)
	}
}

func (suite *IntegrationTestSuite) TestTenantLifecycle() {
	// Test creating a tenant
	createReq := models.CreateTenantRequest{Name: "Test Tenant"}
	reqBody, _ := json.Marshal(createReq)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusCreated, w.Code)

	var tenant models.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &tenant)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), "Test Tenant", tenant.Name)
	assert.NotEmpty(suite.T(), tenant.ID)

	tenantID := tenant.ID

	// Test getting the tenant
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/tenants/%s", tenantID), nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	// Test listing tenants
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/tenants", nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var tenants []*models.Tenant
	err = json.Unmarshal(w.Body.Bytes(), &tenants)
	suite.Require().NoError(err)
	assert.Len(suite.T(), tenants, 1)

	// Test updating concurrency
	concurrencyReq := models.UpdateConcurrencyRequest{Workers: 5}
	reqBody, _ = json.Marshal(concurrencyReq)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/api/v1/tenants/%s/config/concurrency", tenantID), bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	// Test deleting the tenant
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/api/v1/tenants/%s", tenantID), nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	// Verify tenant is deleted
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/tenants/%s", tenantID), nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusNotFound, w.Code)
}

func (suite *IntegrationTestSuite) TestMessageOperations() {
	// First create a tenant
	createReq := models.CreateTenantRequest{Name: "Message Test Tenant"}
	reqBody, _ := json.Marshal(createReq)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	suite.router.ServeHTTP(w, req)

	var tenant models.Tenant
	json.Unmarshal(w.Body.Bytes(), &tenant)
	tenantID := tenant.ID

	// Test creating messages
	messageReq := models.CreateMessageRequest{
		Payload: json.RawMessage(`{"test": "data", "number": 123}`),
	}
	reqBody, _ = json.Marshal(messageReq)

	for i := 0; i < 5; i++ {
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", fmt.Sprintf("/api/v1/messages/%s", tenantID), bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		suite.router.ServeHTTP(w, req)

		assert.Equal(suite.T(), http.StatusCreated, w.Code)
	}

	// Test getting messages with pagination
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/messages?tenant_id=%s&limit=3", tenantID), nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var paginatedMessages services.PaginatedMessages
	err := json.Unmarshal(w.Body.Bytes(), &paginatedMessages)
	suite.Require().NoError(err)
	assert.Len(suite.T(), paginatedMessages.Data, 3)
	assert.NotNil(suite.T(), paginatedMessages.NextCursor)

	// Test getting next page
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/messages?tenant_id=%s&limit=3&cursor=%s", tenantID, *paginatedMessages.NextCursor), nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &paginatedMessages)
	suite.Require().NoError(err)
	assert.Len(suite.T(), paginatedMessages.Data, 2)
	assert.Nil(suite.T(), paginatedMessages.NextCursor)

	// Test message stats
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/stats/tenants/%s/messages", tenantID), nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var stats models.MessageStats
	err = json.Unmarshal(w.Body.Bytes(), &stats)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), int64(5), stats.TotalMessages)

	// Cleanup
	suite.tenantManager.DeleteTenant(tenantID)
}

func (suite *IntegrationTestSuite) TestConcurrentMessageProcessing() {
	// Create a tenant
	createReq := models.CreateTenantRequest{Name: "Concurrent Test Tenant"}
	reqBody, _ := json.Marshal(createReq)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	suite.router.ServeHTTP(w, req)

	var tenant models.Tenant
	json.Unmarshal(w.Body.Bytes(), &tenant)
	tenantID := tenant.ID

	// Update concurrency to 10 workers
	concurrencyReq := models.UpdateConcurrencyRequest{Workers: 10}
	reqBody, _ = json.Marshal(concurrencyReq)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/api/v1/tenants/%s/config/concurrency", tenantID), bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	// Publish messages to RabbitMQ queue directly to test consumer
	for i := 0; i < 20; i++ {
		payload := fmt.Sprintf(`{"message_id": %d, "data": "test data"}`, i)
		err := suite.rabbitmq.PublishMessage(tenantID, []byte(payload))
		suite.Require().NoError(err)
	}

	// Wait a bit for messages to be processed
	time.Sleep(2 * time.Second)

	// Cleanup
	suite.tenantManager.DeleteTenant(tenantID)
}

func (suite *IntegrationTestSuite) TestHealthEndpoint() {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	suite.Require().NoError(err)
	assert.Equal(suite.T(), "healthy", response["status"])
}
