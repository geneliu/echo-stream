package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	DefaultBufferSize   = 32 * 1024         // 32KB
	MaxDownloadSize     = 100 * 1024 * 1024 // 100MB limit
	DefaultDownloadSize = 2 * 1024 * 1024   // 2MB default
	ServerTimeout       = 30 * time.Second
	ServerPort          = ":8080"
)

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to prevent abuse
	const maxUploadSize = 10 * 1024 * 1024 // 10MB limit
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	defer r.Body.Close()

	// Stream request body directly to discard
	_, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		log.Printf("Upload error: %v", err)
		http.Error(w, "Request too large or processing error", http.StatusRequestEntityTooLarge)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	sizeStr := r.URL.Query().Get("size")
	if sizeStr == "" {
		sizeStr = strconv.Itoa(DefaultDownloadSize)
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		log.Printf("Invalid size parameter: %v", err)
		http.Error(w, "invalid size parameter", http.StatusBadRequest)
		return
	}

	// Validate size bounds
	if size <= 0 || size > MaxDownloadSize {
		log.Printf("Size out of bounds: %d bytes", size)
		http.Error(w, "size must be between 1 and 104857600 bytes", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.WriteHeader(http.StatusOK)

	buf := make([]byte, DefaultBufferSize)
	written := 0

	for written < size {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			log.Printf("Client disconnected during download")
			return
		default:
		}

		toWrite := len(buf)
		if size-written < toWrite {
			toWrite = size - written
		}

		_, err := w.Write(buf[:toWrite])
		if err != nil {
			log.Printf("Write error: %v", err)
			return
		}

		// Flush to ensure data is sent immediately
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		written += toWrite
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("Streaming test server starting on %s", ServerPort)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
