package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oschwald/geoip2-golang"
)

var geoDB *geoip2.Reader

// Config holds application configuration.
type Config struct {
	GeoIPDBPath string
	ListenAddr  string
}

// AppError represents a structured error response.
type AppError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// defaultGeoIPDir is the default directory to search for the GeoIP database.
const defaultGeoIPDir = "/app/data"

// defaultGeoIPFile is the default GeoIP database filename.
const defaultGeoIPFile = "GeoLite2-City.mmdb"

func loadConfig() (Config, error) {
	dbPath := os.Getenv("GEOIP_DB_PATH")
	listenAddr := os.Getenv("LISTEN_ADDR")

	if listenAddr == "" {
		listenAddr = ":8080" // Default listen address
	}

	if dbPath == "" {
		// GEOIP_DB_PATH environment variable is not set.
		// Attempt to use a default path, which aligns with the geoipupdate service volume mount.
		potentialDefaultPath := filepath.Join(defaultGeoIPDir, defaultGeoIPFile)
		log.Printf("GEOIP_DB_PATH not set. Checking default location: %s", potentialDefaultPath)

		if _, err := os.Stat(potentialDefaultPath); err == nil {
			// Default file exists
			log.Printf("Using GeoIP database found at default location: %s", potentialDefaultPath)
			dbPath = potentialDefaultPath
		} else if os.IsNotExist(err) {
			// Default file does not exist
			errMsg := fmt.Sprintf("GEOIP_DB_PATH environment variable is not set, and the default database '%s' was not found in '%s'. Please ensure the database file is available or set GEOIP_DB_PATH.", defaultGeoIPFile, defaultGeoIPDir)
			log.Println(errMsg)
			return Config{}, errors.New(errMsg)
		} else {
			// Some other error occurred when checking for the default file (e.g., permission issues)
			errMsg := fmt.Sprintf("Error checking for default GeoIP database at '%s': %v. Please ensure the path is accessible or set GEOIP_DB_PATH.", potentialDefaultPath, err)
			log.Println(errMsg)
			return Config{}, errors.New(errMsg)
		}
	} else {
		log.Printf("Using GeoIP database path from GEOIP_DB_PATH: %s", dbPath)
	}

	return Config{
		GeoIPDBPath: dbPath,
		ListenAddr:  listenAddr,
	}, nil
}

func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(AppError{Message: message, Code: code})
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if geoDB == nil {
		writeJSONError(w, "GeoIP database not loaded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func lookupHandler(w http.ResponseWriter, r *http.Request) {
	if geoDB == nil {
		log.Println("Error: GeoIP database is not loaded.")
		writeJSONError(w, "GeoIP service not available", http.StatusInternalServerError)
		return
	}

	ipStr := ""
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	if len(pathParts) > 1 && pathParts[1] != "" {
		ipStr = pathParts[1]
	} else {
		remoteAddr := r.RemoteAddr
		host, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			ipStr = host
		} else {
			ipStr = remoteAddr
		}
		if ipStr == "::1" || ipStr == "127.0.0.1" {
			log.Printf("Request IP is local (%s). GeoIP lookup might return limited or no data.", ipStr)
		}
	}

	if ipStr == "" {
		writeJSONError(w, "Could not determine IP address from request", http.StatusBadRequest)
		return
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		writeJSONError(w, fmt.Sprintf("Invalid IP address format: %s", ipStr), http.StatusBadRequest)
		return
	}

	record, err := geoDB.City(ip)
	if err != nil {
		log.Printf("Could not find GeoIP data for IP %s: %v", ip.String(), err)
		writeJSONError(w, fmt.Sprintf("GeoIP data not found for IP: %s", ip.String()), http.StatusNotFound)
		return
	}

	response := map[string]any{
		"ip":           ip.String(),
		"city":         record.City.Names["en"],
		"country_code": record.Country.IsoCode,
		"country_name": record.Country.Names["en"],
		"continent":    record.Continent.Names["en"],
		"latitude":     record.Location.Latitude,
		"longitude":    record.Location.Longitude,
		"time_zone":    record.Location.TimeZone,
		"postal_code":  record.Postal.Code,
	}
	if record.Subdivisions != nil && len(record.Subdivisions) > 0 {
		response["subdivision_name"] = record.Subdivisions[0].Names["en"]
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response for IP %s: %v", ip.String(), err)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := loadConfig()
	if err != nil {
		// loadConfig now logs detailed messages, so a fatal log here is sufficient.
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Attempting to load GeoIP database from: %s", cfg.GeoIPDBPath)
	geoDB, err = geoip2.Open(cfg.GeoIPDBPath)
	if err != nil {
		log.Fatalf("Error opening GeoIP database at %s: %v", cfg.GeoIPDBPath, err)
	}
	defer func() {
		if err := geoDB.Close(); err != nil {
			log.Printf("Error closing GeoIP database: %v", err)
		}
	}()
	log.Println("GeoIP database loaded successfully.")

	mux := http.NewServeMux()
	mux.HandleFunc("/lookup/", lookupHandler)
	mux.HandleFunc("/healthz", healthzHandler)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Could not listen on %s: %v\n", cfg.ListenAddr, err)
		}
	}()
	log.Println("Server started. Press Ctrl+C to shut down.")

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server gracefully stopped.")
}
