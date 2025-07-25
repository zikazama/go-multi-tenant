# Multi-Tenant Messaging System

A high-performance, scalable multi-tenant messaging system built with Go, RabbitMQ, and PostgreSQL. The system features dynamic consumer management, partitioned data storage, configurable concurrency, and comprehensive monitoring.

## Features

- **Multi-Tenant Architecture**: Isolated message processing per tenant
- **Dynamic Consumer Management**: Auto-spawn and auto-stop tenant consumers
- **Partitioned Database**: PostgreSQL table partitioning for optimal performance
- **Configurable Concurrency**: Adjustable worker pools per tenant
- **Graceful Shutdown**: Clean shutdown with transaction completion
- **Cursor Pagination**: Efficient message retrieval with cursor-based pagination
- **Comprehensive Monitoring**: Prometheus metrics and Grafana dashboards
- **API Documentation**: Auto-generated Swagger documentation
- **Integration Testing**: Docker-based testing with real services

## Quick Start

### Prerequisites

- Go 1.21+
- Docker and Docker Compose
- Make (optional, for convenience commands)

### Development Setup

1. **Clone and setup the project**:
   ```bash
   git clone <repository-url>
   cd jatis
   make deps
   ```

2. **Start infrastructure services**:
   ```bash
   make docker-up
   ```

3. **Run the application**:
   ```bash
   make run
   ```

4. **Access the services**:
   - API: http://localhost:8080
   - Swagger UI: http://localhost:8080/swagger/index.html
   - RabbitMQ Management: http://localhost:15672 (guest/guest)
   - Prometheus: http://localhost:9090
   - Grafana: http://localhost:3000 (admin/admin)

### Using Docker Compose

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

## API Endpoints

> **Note:** In all example commands, replace `{tenant_id}` and `{id}` with the actual UUID values returned from the API (e.g., `a2536bf8-ac35-4895-b54a-d6657061eff6`).

### Tenants

- `POST /api/v1/tenants` - Create a new tenant
- `GET /api/v1/tenants` - List all tenants
- `GET /api/v1/tenants/{id}` - Get tenant by ID
- `DELETE /api/v1/tenants/{id}` - Delete tenant
- `PUT /api/v1/tenants/{id}/config/concurrency` - Update worker concurrency

### Messages

- `GET /api/v1/messages?tenant_id={id}&cursor={cursor}&limit={limit}` - Get messages with pagination
- `POST /api/v1/messages/{tenant_id}` - Create a message
- `GET /api/v1/messages/{id}` - Get message by ID
- `DELETE /api/v1/messages/{id}` - Delete message

### Statistics

- `GET /api/v1/stats/tenants/{id}/messages` - Get message statistics for a tenant

### System

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics

## Configuration

The application can be configured via `config.yaml` or environment variables:

```yaml
rabbitmq:
  url: amqp://guest:guest@localhost:5672/
database:
  url: postgres://postgres:postgres@localhost:5432/jatis?sslmode=disable
workers: 3  # Default worker count per tenant
```

### Environment Variables

- `RABBITMQ_URL` - RabbitMQ connection URL
- `DATABASE_URL` - PostgreSQL connection URL

## Examples

### Creating a Tenant

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{"name": "My Company"}'
```

### Creating a Message

```bash
curl -X POST http://localhost:8080/api/v1/messages/a2536bf8-ac35-4895-b54a-d6657061eff6 \
  -H "Content-Type: application/json" \
  -d '{"payload": {"type": "order", "data": {"id": 123, "amount": 99.99}}}'
```

### Getting Messages with Pagination

```bash
curl "http://localhost:8080/api/v1/messages?tenant_id=a2536bf8-ac35-4895-b54a-d6657061eff6&limit=10"
```

### Updating Concurrency

```bash
curl -X PUT http://localhost:8080/api/v1/tenants/{tenant_id}/config/concurrency \
  -H "Content-Type: application/json" \
  -d '{"workers": 10}'
```

## Testing

### Unit Tests

```bash
make test
```

### Integration Tests

```bash
make test-integration
```

### Test Coverage

```bash
make test-coverage
```

The integration tests use Docker containers for PostgreSQL and RabbitMQ to ensure tests run against real services.

## Monitoring

### Metrics

The application exposes Prometheus metrics at `/metrics`:

- `http_requests_total` - Total HTTP requests
- `http_request_duration_seconds` - HTTP request duration
- `active_tenants_total` - Number of active tenants
- `messages_processed_total` - Messages processed per tenant
- `message_queue_depth` - Queue depth per tenant
- `active_workers_total` - Active workers per tenant

### Dashboards

Grafana dashboards are available for:
- Application performance
- Message throughput
- Queue monitoring
- System resources

## Database Schema

### Tenants Table

```sql
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Partitioned Messages Table

```sql
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
) PARTITION BY LIST (tenant_id);
```

### Tenant Configuration

```sql
CREATE TABLE tenant_configs (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    workers INTEGER NOT NULL DEFAULT 3,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Performance Considerations

### Database Partitioning

Each tenant gets its own partition in the messages table, which provides:
- Improved query performance
- Easier data management
- Better scalability

### Worker Pools

Configurable worker pools per tenant allow for:
- Optimal resource utilization
- Tenant-specific performance tuning
- Dynamic scaling based on load

### Connection Pooling

The application uses connection pooling for both PostgreSQL and RabbitMQ to:
- Reduce connection overhead
- Improve throughput
- Handle concurrent requests efficiently

## Deployment

### Production Build

```bash
make build-linux
```

### Docker Build

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o jatis

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/jatis .
COPY --from=builder /app/config.yaml .
CMD ["./jatis"]
```

### Environment Variables for Production

```bash
export DATABASE_URL="postgres://user:pass@db:5432/jatis?sslmode=require"
export RABBITMQ_URL="amqp://user:pass@rabbitmq:5672/"
```

## Development

### Project Structure

```
jatis/
├── cmd/                    # Command line tools
├── internal/
│   ├── api/               # HTTP handlers and routes
│   ├── config/            # Configuration management
│   ├── database/          # Database layer
│   ├── messaging/         # RabbitMQ integration
│   ├── metrics/           # Prometheus metrics
│   ├── models/            # Data models
│   └── services/          # Business logic
├── tests/                 # Integration tests
├── monitoring/            # Monitoring configuration
├── docs/                  # Documentation
├── config.yaml           # Default configuration
├── docker-compose.yml     # Development environment
├── Makefile              # Build and development commands
└── main.go               # Application entry point
```

### Adding New Features

1. Add models in `internal/models/`
2. Implement business logic in `internal/services/`
3. Add HTTP handlers in `internal/api/`
4. Add tests in `tests/`
5. Update documentation

### Code Quality

```bash
# Format code
make fmt

# Lint code
make lint

# Install development tools
make install-tools
```

## Troubleshooting

### Common Issues

1. **Database connection failed**
   - Check PostgreSQL is running
   - Verify connection string
   - Ensure database exists

2. **RabbitMQ connection failed**
   - Check RabbitMQ is running
   - Verify credentials
   - Check network connectivity

3. **Tests failing**
   - Ensure Docker is running
   - Check available ports (5432, 5672)
   - Verify test database permissions

### Logs

Application logs include:
- HTTP request logs
- Database operation logs
- RabbitMQ connection logs
- Worker pool activity
- Error details with stack traces

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run the test suite
6. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For support and questions:
- Create an issue in the repository
- Check the documentation
- Review the API documentation at `/swagger/`# go-multi-tenant
# go-multi-tenant
