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
