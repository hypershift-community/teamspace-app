# Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend

# Copy frontend package files and install dependencies
COPY frontend/package*.json ./
RUN npm ci

# Copy the rest of the frontend code
COPY frontend/ ./

# Build the frontend
RUN npm run build

# Build backend
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app/backend

# Install necessary build tools
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy the rest of the backend code
COPY backend/ ./

# Build the backend explicitly for linux/amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o teamspace-app ./cmd/server

# Final stage
FROM alpine:3.19
WORKDIR /app

# Install CA certificates for HTTPS requests
RUN apk add --no-cache ca-certificates tzdata

# Copy the backend binary from the builder stage
COPY --from=backend-builder /app/backend/teamspace-app /app/teamspace-app

# Create frontend directory structure
RUN mkdir -p /app/frontend/dist

# Copy the frontend build from the frontend builder
COPY --from=frontend-builder /app/frontend/dist/ /app/frontend/dist/

# Create directory for config
RUN mkdir -p /app/config

# Set executable permissions explicitly
RUN chmod +x /app/teamspace-app

# Expose the port the app runs on
EXPOSE 8080

# Command to run the application
ENTRYPOINT ["/app/teamspace-app"]
CMD ["--config", "/app/config/config.json"] 