package server

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/usb-wiper/internal/config"
	"github.com/usb-wiper/internal/wipe"
)

//go:embed static/*
var staticFiles embed.FS

// Server is the main HTTP server for the USB Wiper application.
type Server struct {
	port   string
	config *config.Manager
	sseHub *SSEHub
	jobs   *jobManager
	server *http.Server
}

// New creates a new Server instance.
func New(port string, unsafeAllowAllUSB bool) *Server {
	return &Server{
		port:   port,
		config: config.New(unsafeAllowAllUSB),
		sseHub: NewSSEHub(),
		jobs:   newJobManager(),
	}
}

// Start begins listening and serving HTTP requests.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Static files from embedded filesystem
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	// Serve index.html at root
	mux.HandleFunc("GET /", s.handleIndex)

	// API routes
	mux.HandleFunc("GET /api/devices", s.handleGetDevices)
	mux.HandleFunc("GET /api/health", s.handleGetHealth)
	mux.HandleFunc("POST /api/wipe", s.handlePostWipe)
	mux.HandleFunc("POST /api/cancel", s.handlePostCancel)
	mux.HandleFunc("GET /api/job", s.handleGetJob)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/config", s.handlePostConfig)
	mux.HandleFunc("GET /api/events", s.handleSSE)

	// Health check
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Apply middleware
	handler := withLogging(withRecovery(withCORS(mux)))

	s.server = &http.Server{
		Addr:         ":" + s.port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Disable write timeout for SSE
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("server listening on :%s", s.port)
	log.Println("open http://localhost:" + s.port)

	// Start server
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		log.Println("shutting down server")
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev wipe.ProgressEvent) {
	data, _ := json.Marshal(ev)
	msg := "data: " + string(data) + "\n\n"
	w.Write([]byte(msg))
	flusher.Flush()
}
