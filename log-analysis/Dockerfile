# Stage 1: Build the Go binary
FROM golang:1.21 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o log-analysis ./cmd/rest_api

# Stage 2: Create a minimal image to run the binary
FROM ubuntu:22.04 as base

# Set the working directory inside the container
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/log-analysis .

EXPOSE 8094

ENTRYPOINT ["/app/log-analysis"]