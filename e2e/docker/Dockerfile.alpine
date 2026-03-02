
FROM golang:alpine

# Install test dependencies
RUN apk add --no-cache curl jq unzip bash

WORKDIR /app

# Copy dependency files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN go build -o /usr/local/bin/jend ./cmd/jend

# Default command (can be overridden)
CMD ["jend", "--help"]
