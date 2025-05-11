# IP Lookup Service

A simple and efficient Go-based HTTP service that provides geolocation information for IP addresses using the MaxMind GeoLite2-City database.

## Prerequisites

- **Go**: Version 1.18 or higher.
- **GeoLite2-City Database**: You need to download the `GeoLite2-City.mmdb` file from [MaxMind](https://www.maxmind.com/en/geolite2/signup). This service expects the database file to be present.

## Installation & Setup

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/ali-issa/ip-lookup
    cd ip-lookup
    ```

2.  **Download the GeoLite2-City Database:**
    Obtain the `GeoLite2-City.mmdb` file from MaxMind and place it in a directory accessible by the application. By default, the application looks for it at `/app/data/GeoLite2-City.mmdb` (common in containerized environments) or a path specified by the `GEOIP_DB_PATH` environment variable.

3.  **Build the application:**
    ```bash
    go build -o ip-lookup-service main.go
    ```

## Releases

Pre-compiled binaries for various operating systems and architectures are available on the [GitHub Releases page](https://github.com/ali-issa/ip-lookup/releases). You can download the appropriate binary for your system instead of building from source.

## Configuration

The service is configured using environment variables:

- `GEOIP_DB_PATH`: (Required unless default path is used) The absolute path to your `GeoLite2-City.mmdb` file.
  - If not set, the application will attempt to load the database from `/app/data/GeoLite2-City.mmdb`.
  - Example: `export GEOIP_DB_PATH="/path/to/your/GeoLite2-City.mmdb"`
- `LISTEN_ADDR`: (Optional) The address and port on which the server should listen.
  - Defaults to `:8080`.
  - Example: `export LISTEN_ADDR=":9000"`

## Running the Service

Once built and configured, you can run the service:

```bash
./ip-lookup-service
```

**Example with environment variables:**

```bash
export GEOIP_DB_PATH="/opt/geoip/GeoLite2-City.mmdb"
export LISTEN_ADDR=":8080"
./ip-lookup-service
```

The server will start, and log messages will indicate if the GeoIP database was loaded successfully and the address it's listening on.

## Docker

A pre-built Docker image is available on Docker Hub: `ali-issa/ip-lookup`.
The image is based on Alpine Linux and uses the binary from the GitHub releases.

The `ip-lookup` Docker container expects the GeoLite2 database to be available at `/geoipdb/GeoLite2-City.mmdb` by default. This path is configurable via the `GEOIP_DB_PATH` environment variable if needed.

The recommended way to run this service with Docker is by using `docker-compose` along with MaxMind's official `geoipupdate` container. This automates the download and periodic refresh of the GeoLite2 database, which the `ip-lookup` service will then use.

Here’s how to set it up:

1.  **Create a `docker-compose.yml` file:**

    ```yaml
    version: "3.8"

    services:
      geoipupdate:
        container_name: geoipupdate
        image: ghcr.io/maxmind/geoipupdate
        restart: unless-stopped
        environment:
          # Replace with your actual MaxMind account ID and license key
          - GEOIPUPDATE_ACCOUNT_ID=YOUR_ACCOUNT_ID
          - GEOIPUPDATE_LICENSE_KEY=YOUR_LICENSE_KEY
          - GEOIPUPDATE_EDITION_IDS=GeoLite2-City # We only need the City database
          - GEOIPUPDATE_FREQUENCY=72 # How often to check for updates (in hours)
        volumes:
          - geoip_data:/usr/share/GeoIP # geoipupdate writes here

      ip-lookup:
        container_name: ip-lookup
        image: ali-issa/ip-lookup:latest # Or a specific version
        restart: unless-stopped
        depends_on:
          - geoipupdate
        ports:
          - "8080:8080" # Map host port 8080 to container port 8080
        volumes:
          - geoip_data:/geoipdb # ip-lookup reads from here
        environment:
          # GEOIP_DB_PATH is already set to /geoipdb/GeoLite2-City.mmdb in the Dockerfile
          # LISTEN_ADDR can be overridden here if needed, e.g., - LISTEN_ADDR=:9000
          - LISTEN_ADDR=:8080
        # Add healthcheck if desired
        # healthcheck:
        #   test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
        #   interval: 30s
        #   timeout: 10s
        #   retries: 3

    volumes:
      geoip_data: # This named volume is shared between the two services
        driver: local
    ```

2.  **Sign up for a MaxMind Account:**
    You need a MaxMind account and a license key to use `geoipupdate`. You can typically get these from the [MaxMind website](https://www.maxmind.com/en/geolite2/signup). The free GeoLite2 databases require account signup.

3.  **Configure Environment Variables:**
    Replace `YOUR_ACCOUNT_ID` and `YOUR_LICENSE_KEY` in the `docker-compose.yml` file with your actual MaxMind credentials. Alternatively, you can use a `.env` file in the same directory as your `docker-compose.yml`:

    ```env
    GEOIPUPDATE_ACCOUNT_ID=YOUR_ACCOUNT_ID
    GEOIPUPDATE_LICENSE_KEY=YOUR_LICENSE_KEY
    ```

    And then reference them in `docker-compose.yml`:

    ```yaml
    # ... inside geoipupdate service environment:
    - GEOIPUPDATE_ACCOUNT_ID=${GEOIPUPDATE_ACCOUNT_ID}
    - GEOIPUPDATE_LICENSE_KEY=${GEOIPUPDATE_LICENSE_KEY}
    # ...
    ```

4.  **Run Docker Compose:**
    ```bash
    docker-compose up -d
    ```
    This will start both services. The `geoipupdate` service will download the `GeoLite2-City.mmdb` into the shared `geoip_data` volume, and `ip-lookup` will read it from there.

## API Endpoints

### 1. Lookup IP Address

- **Endpoint**: `/lookup/{ip_address}`
- **Method**: `GET`
- **Description**: Retrieves geolocation data for the specified IP address.
- **Example**:
  ```bash
  curl http://localhost:8080/lookup/8.8.8.8
  ```
- **Success Response (200 OK)**:
  ```json
  {
    "ip": "8.8.8.8",
    "city": "Mountain View",
    "country_code": "US",
    "country_name": "United States",
    "continent": "North America",
    "latitude": 37.422,
    "longitude": -122.084,
    "time_zone": "America/Los_Angeles",
    "postal_code": "94043",
    "subdivision_name": "California" // Present if available
  }
  ```
- **Error Responses**:
  - `400 Bad Request`: If the IP address format is invalid.
    ```json
    {
      "message": "Invalid IP address format: X.X.X.X",
      "code": 400
    }
    ```
  - `404 Not Found`: If GeoIP data is not found for the IP.
    ```json
    {
      "message": "GeoIP data not found for IP: X.X.X.X",
      "code": 404
    }
    ```

### 2. Lookup Client's IP Address

- **Endpoint**: `/lookup/` or `/lookup`
- **Method**: `GET`
- **Description**: Retrieves geolocation data for the IP address of the client making the request.
- **Example**:
  ```bash
  curl http://localhost:8080/lookup/
  ```
- **Success Response (200 OK)**: Same format as `/lookup/{ip_address}`.
- **Error Responses**:
  - `400 Bad Request`: If the client's IP could not be determined.

### 3. Health Check

- **Endpoint**: `/healthz`
- **Method**: `GET`
- **Description**: Checks the health of the service, primarily if the GeoIP database is loaded.
- **Example**:
  ```bash
  curl http://localhost:8080/healthz
  ```
- **Success Response (200 OK)**:
  ```json
  {
    "status": "ok"
  }
  ```
- **Error Response (500 Internal Server Error)**: If the GeoIP database is not loaded.
  ```json
  {
    "message": "GeoIP database not loaded",
    "code": 500
  }
  ```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request or open an issue for bugs, feature requests, or improvements.

1.  Fork the repository.
2.  Create your feature branch (`git checkout -b feature/AmazingFeature`).
3.  Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4.  Push to the branch (`git push origin feature/AmazingFeature`).
5.  Open a Pull Request.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
