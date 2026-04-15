# --- Build Stage ---
FROM golang:1.22-alpine AS builder

# Set necessary environment variables for CGO compilation if needed, though usually not for simple Go apps
ENV CGO_ENABLED=0

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod ./
RUN go mod download

# Copy the source code
COPY main.go .

# Build the application statically linked for maximum compatibility
RUN go build -ldflags="-s -w" -o /koito-obs-overlay .

# --- Final Stage ---
# Use a minimal base image for the final deployment
FROM alpine:latest

WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /koito-obs-overlay .

# Expose the port the application listens on (8080)
EXPOSE 8080

# Command to run the application
CMD ["./koito-obs-overlay"]
