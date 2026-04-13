# Stage 1: Build & Test
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and static assets
COPY . .

# Run tests
RUN go test -v ./...

# Compile the binary statically
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o uplarr ./cmd/uplarr

# Stage 2: Final minimal image
FROM alpine:3.23

# Install CA certificates and tzdata for secure connections and time handling
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/uplarr .

# Expose the default web port
EXPOSE 8080

# Run the application
CMD ["./uplarr"]
