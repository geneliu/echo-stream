package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultBufferSize   = 32 * 1024         // 32KB
	MaxDownloadSize     = 100 * 1024 * 1024 // 100MB limit
	MaxUploadSize       = 32 * 1024 * 1024  // 32MB limit
	DefaultDownloadSize = 2 * 1024 * 1024   // 2MB default
	ServerTimeout       = 30 * time.Second
	ServerPort          = ":8080"
)

// getClientIP extracts the real client IP from various headers
func getClientIP(r *http.Request) string {
	// Check Cloudflare/Load balancer headers first
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(ip, ",")
		return strings.TrimSpace(ips[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Fall back to remote address
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Log incoming request
	clientIP := getClientIP(r)
	log.Printf("UPLOAD REQUEST: Client=%s Method=%s URL=%s ContentLength=%d UserAgent=%s",
		clientIP, r.Method, r.URL.Path, r.ContentLength, r.UserAgent())

	// Limit request body size to prevent abuse
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
	defer r.Body.Close()

	// Stream request body directly to discard
	bytesRead, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		log.Printf("UPLOAD ERROR: Client=%s Error=%v BytesRead=%d", clientIP, err, bytesRead)
		http.Error(w, "Request too large or processing error", http.StatusRequestEntityTooLarge)
		return
	}

	log.Printf("UPLOAD SUCCESS: Client=%s BytesReceived=%d", clientIP, bytesRead)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	sizeStr := r.URL.Query().Get("size")
	if sizeStr == "" {
		sizeStr = strconv.Itoa(DefaultDownloadSize)
	}

	log.Printf("DOWNLOAD REQUEST: Client=%s Method=%s URL=%s RequestedSize=%s UserAgent=%s",
		clientIP, r.Method, r.URL.Path, sizeStr, r.UserAgent())

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		log.Printf("DOWNLOAD ERROR: Client=%s InvalidSize=%s Error=%v", clientIP, sizeStr, err)
		http.Error(w, "invalid size parameter", http.StatusBadRequest)
		return
	}

	// Validate size bounds
	if size <= 0 || size > MaxDownloadSize {
		log.Printf("DOWNLOAD ERROR: Client=%s SizeOutOfBounds=%d Min=1 Max=%d", clientIP, size, MaxDownloadSize)
		http.Error(w, "size must be between 1 and 104857600 bytes", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.WriteHeader(http.StatusOK)

	buf := make([]byte, DefaultBufferSize)
	written := 0

	log.Printf("DOWNLOAD START: Client=%s TotalSize=%d", clientIP, size)

	for written < size {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			log.Printf("DOWNLOAD DISCONNECTED: Client=%s BytesSent=%d Total=%d", clientIP, written, size)
			return
		default:
		}

		toWrite := len(buf)
		if size-written < toWrite {
			toWrite = size - written
		}

		_, err := w.Write(buf[:toWrite])
		if err != nil {
			log.Printf("DOWNLOAD WRITE ERROR: Client=%s BytesSent=%d Error=%v", clientIP, written, err)
			return
		}

		// Flush to ensure data is sent immediately
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		written += toWrite
	}

	log.Printf("DOWNLOAD SUCCESS: Client=%s BytesSent=%d", clientIP, written)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	log.Printf("HEALTH CHECK: Client=%s Method=%s URL=%s UserAgent=%s",
		clientIP, r.Method, r.URL.Path, r.UserAgent())

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("healthy"))
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/download", downloadHandler)
	mux.HandleFunc("/health", healthHandler)

	server := &http.Server{
		Addr:         ServerPort,
		Handler:      mux,
		ReadTimeout:  ServerTimeout,
		WriteTimeout: ServerTimeout,
		IdleTimeout:  ServerTimeout,
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("SERVER STARTING: Port=%s Timeout=%v PID=%d", ServerPort, ServerTimeout, os.Getpid())
	log.Printf("ENDPOINTS: UPLOAD=/upload DOWNLOAD=/download HEALTH=/health")

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("SERVER ERROR: Failed to start: %v", err)
		}
	}()

	<-stop
	log.Println("SERVER SHUTDOWN: Received termination signal")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("SERVER SHUTDOWN ERROR: Forced shutdown: %v", err)
	}

	log.Println("SERVER STOPPED: Exited gracefully")
}
