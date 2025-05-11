# Use a lightweight base image
FROM alpine:latest

# Arguments for release version and architecture
ARG RELEASE_VERSION=v0.1.0
ARG TARGETARCH=amd64
ARG BINARY_NAME=ip-lookup

# Install curl and ca-certificates for downloading the binary
RUN apk add --no-cache curl ca-certificates

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Create app directory
WORKDIR /app
RUN chown -R appuser:appgroup /app && \
    chmod -R 755 /app
# Note: /app/data directory creation removed as DB will come from a shared volume at /geoipdb

# Download the binary from GitHub releases
# Note: GitHub release asset names might be ip-lookup-linux-amd64, ip-lookup-linux-arm64 etc.
# Ensure BINARY_NAME_SUFFIX matches the release asset naming convention.
RUN BINARY_FILENAME="${BINARY_NAME}-linux-${TARGETARCH}" && \
    if [ "${TARGETARCH}" = "amd64" ]; then \
        BINARY_FILENAME="${BINARY_NAME}-linux-amd64"; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
        BINARY_FILENAME="${BINARY_NAME}-linux-arm64"; \
    else \
        echo "Unsupported TARGETARCH: ${TARGETARCH}" && exit 1; \
    fi && \
    curl -sSL "https://github.com/ali-issa/ip-lookup/releases/download/${RELEASE_VERSION}/${BINARY_FILENAME}" -o /app/ip-lookup-service && \
    chmod +x /app/ip-lookup-service

# Set user
USER appuser

# Expose the default port (as per README)
EXPOSE 8080

# Environment variables
# Default path for DB from shared volume, used when GEOIP_DB_PATH is not overridden
ENV GEOIP_DB_PATH="/geoipdb/GeoLite2-City.mmdb"
ENV LISTEN_ADDR=":8080"

# Volume for GeoIP database, intended to be shared with a geoipupdate container
VOLUME /geoipdb

# Entrypoint
ENTRYPOINT ["/app/ip-lookup-service"]
