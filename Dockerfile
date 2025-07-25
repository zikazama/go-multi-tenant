# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates (needed for go mod download)
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Create appuser
ENV USER=appuser
ENV UID=10001

# Create a minimal passwd file for the appuser
RUN adduser \    
    --disabled-password \    
    --gecos "" \    
    --home "/nonexistent" \    
    --shell "/sbin/nologin" \    
    --no-create-home \    
    --uid "${UID}" \    
    "${USER}"

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o jatis .

# Final stage
FROM scratch

# Import from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy our static executable
COPY --from=builder /build/jatis /jatis

# Copy configuration
COPY --from=builder /build/config.yaml /config.yaml

# Use an unprivileged user
USER appuser:appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/jatis", "--health-check"] || exit 1

# Run the binary
ENTRYPOINT ["/jatis"]