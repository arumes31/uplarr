# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and static assets
COPY . .

# Compile the binary statically
RUN CGO_ENABLED=0 GOOS=linux go build -o uplarr .

# Stage 2: Final minimal image
FROM alpine:latest

# Install CA certificates for secure connections
RUN apk --no-header add ca-certificates

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/uplarr .

# Expose the default web port
EXPOSE 8080

# Run the application
CMD ["./uplarr"]
