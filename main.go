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
	GeoIPDBPath              string
	ListenAddr               string
	AllowedCORSAccessOrigins []string
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

	allowedOriginsEnv := os.Getenv("ALLOWED_CORS_ORIGINS")
	var allowedOriginsList []string
	if allowedOriginsEnv != "" {
		allowedOriginsList = strings.Split(allowedOriginsEnv, ",")
		for i, origin := range allowedOriginsList {
			allowedOriginsList[i] = strings.TrimSpace(origin)
		}
		log.Printf("Allowed CORS origins: %v", allowedOriginsList)
	} else {
		log.Println("ALLOWED_CORS_ORIGINS not set. CORS headers will not be added.")
	}

	return Config{
		GeoIPDBPath:              dbPath,
		ListenAddr:               listenAddr,
		AllowedCORSAccessOrigins: allowedOriginsList,
	}, nil
}

// corsMiddleware handles CORS headers for incoming requests.
func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		isAllowed := false

		if len(allowedOrigins) == 0 || requestOrigin == "" {
			// If no origins configured or no Origin header, proceed without CORS headers
			next.ServeHTTP(w, r)
			return
		}

		// Check if wildcard '*' is configured
		hasWildcard := false
		for _, configuredOrigin := range allowedOrigins {
			if configuredOrigin == "*" {
				hasWildcard = true
				break
			}
		}

		if hasWildcard {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			isAllowed = true
		} else {
			for _, configuredOrigin := range allowedOrigins {
				if configuredOrigin == requestOrigin {
					w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
					w.Header().Add("Vary", "Origin") // Important for caching proxies
					isAllowed = true
					break
				}
			}
		}

		if isAllowed {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			// Only set Allow-Credentials if not using wildcard for origin, as per spec
			if !hasWildcard && requestOrigin != "" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}


			// Handle preflight OPTIONS requests
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight for 1 day
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// If origin is not allowed and it's a preflight, the browser will block it.
		// For actual requests, if not allowed, no CORS headers are set, and browser blocks.
		next.ServeHTTP(w, r)
	})
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

func rootHandler(w http.ResponseWriter, r *http.Request) {
	// Prevent non-root paths from being handled here if mux is configured loosely
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{
		"message":       "Welcome to the IP Lookup Service. Please use the /lookup endpoint to find GeoIP information.",
		"example_usage": "/lookup/8.8.8.8 or /lookup/",
	}
	json.NewEncoder(w).Encode(response)
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
		// Try X-Forwarded-For first. This header can contain a comma-separated list of IPs.
		// The first IP is typically the original client IP.
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			ips := strings.Split(xff, ",")
			// Trim whitespace from the first IP in the list.
			firstIP := strings.TrimSpace(ips[0])
			if firstIP != "" {
				ipStr = firstIP
			}
		}

		// If X-Forwarded-For is not present or didn't yield an IP, try X-Real-IP.
		// X-Real-IP usually contains a single IP, the original client IP.
		if ipStr == "" {
			xri := r.Header.Get("X-Real-IP")
			if xri != "" {
				ipStr = strings.TrimSpace(xri)
			}
		}

		// Fallback to RemoteAddr if the headers are not present or did not provide an IP.
		// This is less likely when behind a properly configured proxy.
		if ipStr == "" {
			remoteAddr := r.RemoteAddr
			host, _, err := net.SplitHostPort(remoteAddr)
			if err == nil {
				ipStr = host
			} else {
				// If SplitHostPort fails (e.g., for Unix domain sockets or non-standard formats),
				// use RemoteAddr directly.
				ipStr = remoteAddr
			}
		}

		// Log if the determined IP is local, as GeoIP lookup might be limited.
		if ipStr == "::1" || ipStr == "127.0.0.1" {
			log.Printf("Request IP is local (%s) after checking proxy headers. GeoIP lookup might return limited or no data.", ipStr)
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
	mux.HandleFunc("/", rootHandler) // Handle the root path
	mux.HandleFunc("/lookup/", lookupHandler)
	mux.HandleFunc("/healthz", healthzHandler)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           corsMiddleware(mux, cfg.AllowedCORSAccessOrigins), // Apply CORS middleware
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
