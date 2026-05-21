package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/usb-wiper/internal/device"
	"github.com/usb-wiper/internal/format"
	"github.com/usb-wiper/internal/wipe"
)

var (
	currentJob   *wipeJobState
	currentJobMu sync.Mutex
)

type wipeJobState struct {
	Job      *wipe.WipeJob
	progress chan wipe.ProgressEvent
	cancel   context.CancelFunc
}

func (s *Server) handleGetDevices(w http.ResponseWriter, r *http.Request) {
	cfg := s.config.Get()
	devices, err := device.ListUSBDevices(cfg.UnsafeAllowAllUSB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if devices == nil {
		devices = []device.Device{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
	})
}

func (s *Server) handleGetHealth(w http.ResponseWriter, r *http.Request) {
	devicePath := r.URL.Query().Get("device")
	if devicePath == "" {
		writeError(w, http.StatusBadRequest, "device query parameter required")
		return
	}

	health, err := device.GetHealth(devicePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, health)
}

func (s *Server) handlePostWipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device     string `json:"device"`
		AutoFormat bool   `json:"autoFormat"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Device == "" {
		writeError(w, http.StatusBadRequest, "device field required")
		return
	}

	// Server-side safety re-validation
	cfg := s.config.Get()
	if err := device.IsSafeToWipe(req.Device, cfg.UnsafeAllowAllUSB); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check if a job is already running
	currentJobMu.Lock()
	if currentJob != nil && currentJob.Job.Status == wipe.StatusRunning {
		currentJobMu.Unlock()
		writeError(w, http.StatusConflict, "a wipe job is already in progress")
		return
	}

	// Update auto-format config from request
	s.config.SetAutoFormat(req.AutoFormat)

	// Create new job
	progress := make(chan wipe.ProgressEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())

	job := &wipe.WipeJob{
		DevicePath: req.Device,
		Status:     wipe.StatusRunning,
		StartedAt:  time.Now(),
	}

	currentJob = &wipeJobState{
		Job:      job,
		progress: progress,
		cancel:   cancel,
	}
	currentJobMu.Unlock()

	// Run wipe in background
	go func() {
		defer close(progress)

		err := wipe.Wipe(ctx, req.Device, progress, cfg.UnsafeAllowAllUSB)
		now := time.Now()

		currentJobMu.Lock()
		if err != nil {
			if ctx.Err() == context.Canceled {
				job.Status = wipe.StatusCancelled
			} else {
				job.Status = wipe.StatusFailed
				job.Error = err.Error()
			}
		} else {
			job.Status = wipe.StatusCompleted

			// Auto-format if requested
			if req.AutoFormat {
				log.Printf("auto-formatting %s as FAT32", req.Device)
				if formatErr := format.FormatFAT32(req.Device, cfg.UnsafeAllowAllUSB); formatErr != nil {
					log.Printf("auto-format failed: %v", formatErr)
					job.Status = wipe.StatusFailed
					job.Error = "Wipe completed but format failed: " + formatErr.Error()
				}
			}
		}
		job.FinishedAt = now
		currentJobMu.Unlock()
	}()

	// Pipe progress to SSE hub
	go func() {
		for ev := range progress {
			currentJobMu.Lock()
			if currentJob != nil && currentJob.Job == job {
				job.TotalBytes = ev.TotalBytes
				job.BytesWritten = ev.BytesWritten
			}
			currentJobMu.Unlock()
			s.sseHub.Broadcast(ev)
		}

		// Send final event
		s.sseHub.Broadcast(wipe.ProgressEvent{
			DevicePath:   req.Device,
			Status:       job.Status,
			Message:      "Wipe " + job.Status,
			Timestamp:    time.Now(),
			TotalBytes:   job.TotalBytes,
			BytesWritten: job.BytesWritten,
			Percent:      100,
		})
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "started",
		"device": req.Device,
	})
}

func (s *Server) handlePostCancel(w http.ResponseWriter, r *http.Request) {
	currentJobMu.Lock()
	defer currentJobMu.Unlock()

	if currentJob == nil || currentJob.Job.Status != wipe.StatusRunning {
		writeError(w, http.StatusBadRequest, "no active wipe job")
		return
	}

	currentJob.cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	currentJobMu.Lock()
	defer currentJobMu.Unlock()

	if currentJob == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "idle",
		})
		return
	}

	writeJSON(w, http.StatusOK, currentJob.Job)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.config.Get())
}

func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	var cfg struct {
		AutoFormat bool `json:"autoFormat"`
	}

	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	s.config.SetAutoFormat(cfg.AutoFormat)
	writeJSON(w, http.StatusOK, s.config.Get())
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.sseHub.Subscribe()
	defer s.sseHub.Unsubscribe(ch)

	// Send current job state on connect
	currentJobMu.Lock()
	if currentJob != nil {
		ev := wipe.ProgressEvent{
			DevicePath:   currentJob.Job.DevicePath,
			BytesWritten: currentJob.Job.BytesWritten,
			TotalBytes:   currentJob.Job.TotalBytes,
			Status:       currentJob.Job.Status,
			Timestamp:    time.Now(),
		}
		if currentJob.Job.TotalBytes > 0 {
			ev.Percent = float64(currentJob.Job.BytesWritten) / float64(currentJob.Job.TotalBytes) * 100
		}
		writeSSEEvent(w, flusher, ev)
	}
	currentJobMu.Unlock()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(w, flusher, ev)
		}
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev wipe.ProgressEvent) {
	data, _ := json.Marshal(ev)
	msg := "data: " + string(data) + "\n\n"
	w.Write([]byte(msg))
	flusher.Flush()
}
