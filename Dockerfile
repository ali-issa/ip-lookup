# Stage 1: Builder
# Use a golang image that includes Alpine, which has CA certificates
FROM golang:1.24.3-alpine AS builder

# ARG TARGETARCH will be automatically set by Docker Buildx if building for a specific platform.
# Docker Compose with `platform: linux/amd64` will influence this.
ARG TARGETARCH

# Set environment variables for static compilation
ENV CGO_ENABLED=0
ENV GOOS=linux
# Default to amd64 if TARGETARCH is not set, otherwise use TARGETARCH
ENV GOARCH=${TARGETARCH:-amd64}

WORKDIR /src

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the rest of the application source code
COPY . .

# Build the statically linked Go application.
# -s -w flags strip debugging information to reduce binary size.
# Output binary is named ip-lookup-service.
RUN go build -ldflags="-s -w" -o /app/ip-lookup-service main.go

# Stage 2: Final image from scratch
FROM scratch

# Copy CA certificates from the builder stage.
# Go programs often need these for HTTPS requests.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the statically compiled binary from the builder stage.
COPY --from=builder /app/ip-lookup-service /app/ip-lookup-service

# Set the working directory in the final image.
# If your application expects to run from /app, or write files relative to /app.
# For a simple binary like this, running from / is also fine, adjust ENTRYPOINT accordingly.
WORKDIR /app

# Expose the port the application listens on (metadata, does not publish the port)
EXPOSE 8080

# Environment variables needed by the application
ENV GEOIP_DB_PATH="/geoipdb/GeoLite2-City.mmdb"
ENV LISTEN_ADDR=":8080"

# Define the mount point for the GeoIP database.
# This is metadata; the actual volume is mounted via docker-compose.
VOLUME /geoipdb

# Set the entrypoint for the container.
# The binary is now at the root of WORKDIR /app
ENTRYPOINT ["/app/ip-lookup-service"]
