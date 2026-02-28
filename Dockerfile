# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=0.1.0" \
    -o /mcp-server ./cmd/mcp-server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata git

# Create non-root user
RUN adduser -D -u 1000 mcp
USER mcp

COPY --from=builder /mcp-server /usr/local/bin/mcp-server

# Expose HTTP port (for --mode http)
EXPOSE 8080

# Create data directories
RUN mkdir -p /home/mcp/data/contexts /home/mcp/tmp

VOLUME ["/home/mcp/data"]

# Default environment
ENV MCP_STORAGE_PATH=/home/mcp/data/contexts
ENV MCP_TEMP_DIR=/home/mcp/tmp

ENTRYPOINT ["/usr/local/bin/mcp-server"]
