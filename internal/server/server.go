package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/usb-wiper/internal/audit"
	cert2 "github.com/usb-wiper/internal/cert"
	"github.com/usb-wiper/internal/config"
	"github.com/usb-wiper/internal/device"
	"github.com/usb-wiper/internal/metrics"
	"github.com/usb-wiper/internal/persistence"
	"github.com/usb-wiper/internal/presets"
	"github.com/usb-wiper/internal/queue"
	"github.com/usb-wiper/internal/scheduler"
	"github.com/usb-wiper/internal/wipe"
)

//go:embed all:static
var staticFiles embed.FS

// Server is the main HTTP server for the USB Wiper application.
type Server struct {
	port      string
	config    *config.Manager
	sseHub    *SSEHub
	jobs      *queue.Queue
	history   *persistence.Store
	presets   *presets.Store
	schemes   *wipe.SchemeRegistry
	signer    *cert2.Signer
	auditLog  *audit.Logger
	metrics   *metrics.Registry
	scheduler *scheduler.Manager
	server    *http.Server
}

// New creates a new Server instance.
func New(port string, unsafeAllowAllUSB bool, dataDir string) *Server {
	history, err := persistence.New(dataDir)
	if err != nil {
		log.Printf("WARNING: failed to initialize history store: %v (wipes will not be persisted)", err)
		history, _ = persistence.New("")
	}

	presetStore, err := presets.New(dataDir)
	if err != nil {
		log.Printf("WARNING: failed to initialize presets: %v", err)
		presetStore, _ = presets.New("")
	}

	schemeReg := wipe.NewSchemeRegistry()
	sseHub := NewSSEHub()

	jobs := queue.New(queue.Config{
		MaxParallel: 2,
		SSEHub:      sseHub,
		History:     history,
		Schemes:     schemeReg,
		UnsafeAllow: unsafeAllowAllUSB,
	})

	cfg := config.New(unsafeAllowAllUSB, dataDir)

	signer, err := cert2.NewSigner(dataDir)
	if err != nil {
		log.Printf("WARNING: failed to initialize cert signer: %v (certificates will not be signed)", err)
	}

	auditLog, err := audit.New(dataDir)
	if err != nil {
		log.Printf("WARNING: failed to initialize audit log: %v", err)
	}

	// Log server start
	if auditLog != nil {
		auditLog.Log(audit.Event{
			Event: "server_start",
			Actor: "system",
		})
	}

	metricsReg := metrics.New()
	metricsReg.NewCounter("usb_wiper_wipes_total", "Total number of wipe jobs started")
	metricsReg.NewCounter("usb_wiper_wipes_completed", "Total number of wipe jobs completed successfully")
	metricsReg.NewCounter("usb_wiper_wipes_failed", "Total number of wipe jobs that failed")
	metricsReg.NewGauge("usb_wiper_jobs_running", "Number of currently running wipe jobs")
	metricsReg.NewGauge("usb_wiper_jobs_queued", "Number of queued wipe jobs")

	schedMgr, err := scheduler.New(dataDir, nil)
	if err != nil {
		log.Printf("WARNING: failed to initialize scheduler: %v", err)
	}

	return &Server{
		port:      port,
		config:    cfg,
		sseHub:    sseHub,
		jobs:      jobs,
		history:   history,
		presets:   presetStore,
		schemes:   schemeReg,
		signer:    signer,
		auditLog:  auditLog,
		metrics:   metricsReg,
		scheduler: schedMgr,
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

	// ---- API routes ----
	// Device
	mux.HandleFunc("GET /api/devices", s.handleGetDevices)
	mux.HandleFunc("GET /api/health", s.handleGetHealth)

	// History
	mux.HandleFunc("GET /api/history", s.handleGetHistory)

	// Wipe (queue-based)
	mux.HandleFunc("POST /api/wipe", s.handlePostWipe)
	mux.HandleFunc("POST /api/test-wipe", s.handlePostTestWipe)
	mux.HandleFunc("POST /api/cancel", s.handlePostCancel)

	// Jobs
	mux.HandleFunc("GET /api/jobs", s.handleGetJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handlePostCancelJob)

	// Schemes
	mux.HandleFunc("GET /api/schemes", s.handleGetSchemes)

	// Presets
	mux.HandleFunc("GET /api/presets", s.handleGetPresets)
	mux.HandleFunc("POST /api/presets", s.handlePostPresets)
	mux.HandleFunc("PUT /api/presets/{id}", s.handlePutPreset)
	mux.HandleFunc("DELETE /api/presets/{id}", s.handleDeletePreset)

	// Config / Settings
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)   // backward compat
	mux.HandleFunc("POST /api/config", s.handlePostConfig) // backward compat

	// Certificates
	mux.HandleFunc("GET /api/cert/pubkey", s.handleGetPubKey)
	mux.HandleFunc("GET /api/cert/{jobId}/json", s.handleGetCertJSON)
	mux.HandleFunc("GET /api/cert/{jobId}/pdf", s.handleGetCertPDF)
	mux.HandleFunc("POST /api/cert/verify", s.handlePostCertVerify)

	// Audit
	mux.HandleFunc("GET /api/audit", s.handleGetAudit)

	// Schedules
	mux.HandleFunc("GET /api/schedules", s.handleGetSchedules)
	mux.HandleFunc("POST /api/schedules", s.handlePostSchedules)
	mux.HandleFunc("DELETE /api/schedules/{id}", s.handleDeleteSchedule)

	// SSE
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

	// Start separate metrics listener (default 127.0.0.1:9090).
	// Set METRICS_BIND="" to disable entirely.
	metricsBind := os.Getenv("METRICS_BIND")
	if metricsBind == "" {
		metricsBind = "127.0.0.1:9090" // loopback-only by default
	}
	if metricsBind != "off" {
		go func() {
			metricsMux := http.NewServeMux()
			metricsMux.Handle("GET /metrics", s.metrics.Handler())
			ln, err := net.Listen("tcp", metricsBind)
			if err != nil {
				log.Printf("WARNING: metrics listener on %s failed: %v (metrics disabled)", metricsBind, err)
				return
			}
			log.Printf("metrics listening on %s", metricsBind)
			metricsServer := &http.Server{Handler: metricsMux, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
			go func() {
				<-ctx.Done()
				metricsServer.Close()
			}()
			metricsServer.Serve(ln)
		}()
	}

	// Start background device watcher
	go s.watchDevices(ctx)

	// Start job queue dispatcher
	go s.jobs.Start(ctx)

	// Start scheduler (disabled — cron parser is a stub)
	// if s.scheduler != nil {
	// 	go s.scheduler.Start(ctx)
	// }

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
	writeSSEEventID(w, flusher, ev, 0)
}

func writeSSEEventID(w http.ResponseWriter, flusher http.Flusher, ev wipe.ProgressEvent, id uint64) {
	data, _ := json.Marshal(ev)
	var msg string
	if id > 0 {
		if ev.EventType != "" {
			msg = fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", id, ev.EventType, data)
		} else {
			msg = fmt.Sprintf("id: %d\ndata: %s\n\n", id, data)
		}
	} else {
		if ev.EventType != "" {
			msg = "event: " + ev.EventType + "\ndata: " + string(data) + "\n\n"
		} else {
			msg = "data: " + string(data) + "\n\n"
		}
	}
	w.Write([]byte(msg))
	flusher.Flush()
}

// auditEvent logs a security-relevant event with the correlation ID from r.
// Silently no-ops if the audit log is not configured.
func (s *Server) auditEvent(r *http.Request, event, target string, details map[string]interface{}) {
	if s.auditLog == nil {
		return
	}
	s.auditLog.Log(audit.Event{
		Event:     event,
		Actor:     "http",
		Target:    target,
		Details:   details,
		RequestID: requestIDFromContext(r.Context()),
	})
}

// sendRefreshEvent pushes a UI refresh event through the SSE hub.
func (s *Server) sendRefreshEvent() {
	s.sseHub.Broadcast(wipe.ProgressEvent{
		EventType: "refresh",
		Timestamp: time.Now(),
	})
}

// watchDevices periodically scans for USB device changes and broadcasts
// refresh events via SSE when the device list changes (plug/unplug).
func (s *Server) watchDevices(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	var lastDevicePaths string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg := s.config.Get()
			devices, err := device.ListUSBDevices(cfg.UnsafeAllowAllUSB)
			if err != nil {
				continue
			}

			paths := make([]string, 0, len(devices))
			for _, d := range devices {
				paths = append(paths, d.Path)
			}
			sort.Strings(paths)
			current := strings.Join(paths, ",")

			if current != lastDevicePaths && lastDevicePaths != "" {
				s.sseHub.Broadcast(wipe.ProgressEvent{
					EventType: "refresh",
					Timestamp: time.Now(),
					Message:   fmt.Sprintf("device list changed (%d device(s))", len(paths)),
				})
			}
			lastDevicePaths = current
		}
	}
}
