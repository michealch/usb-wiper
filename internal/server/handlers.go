package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/usb-wiper/internal/device"
	"github.com/usb-wiper/internal/format"
	"github.com/usb-wiper/internal/wipe"
)

// jobManager tracks all wipe jobs, keyed by device path.
type jobManager struct {
	mu   sync.Mutex
	jobs map[string]*wipeJobState
}

func newJobManager() *jobManager {
	return &jobManager{
		jobs: make(map[string]*wipeJobState),
	}
}

// get returns the job for a device, nil if not found.
func (jm *jobManager) get(device string) *wipeJobState {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	return jm.jobs[device]
}

// getAll returns a copy of all jobs.
func (jm *jobManager) getAll() map[string]*wipe.WipeJob {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	result := make(map[string]*wipe.WipeJob, len(jm.jobs))
	for k, v := range jm.jobs {
		result[k] = v.Job
	}
	return result
}

// set stores a job for a device. If a job already exists for the device and is
// still running, it returns an error.
func (jm *jobManager) set(device string, job *wipeJobState) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if existing, ok := jm.jobs[device]; ok && existing.Job.Status == wipe.StatusRunning {
		return &jobConflictError{device: device}
	}
	jm.jobs[device] = job
	return nil
}

// remove deletes a job entry. Called after cleanup.
func (jm *jobManager) remove(device string) {
	jm.mu.Lock()
	delete(jm.jobs, device)
	jm.mu.Unlock()
}

// activeDevices returns the set of device paths currently being wiped.
func (jm *jobManager) activeDevices() map[string]bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	active := make(map[string]bool)
	for k, v := range jm.jobs {
		if v.Job.Status == wipe.StatusRunning {
			active[k] = true
		}
	}
	return active
}

type jobConflictError struct {
	device string
}

func (e *jobConflictError) Error() string {
	return "device " + e.device + " is already being wiped"
}

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

	// Attach running wipe status to each device
	activeJobs := s.jobs.getAll()
	for i := range devices {
		if job, ok := activeJobs[devices[i].Path]; ok {
			devices[i].Wiping = job.Status == wipe.StatusRunning
			devices[i].WipeStatus = job.Status
			devices[i].WipePercent = 0
			if job.TotalBytes > 0 {
				devices[i].WipePercent = float64(job.BytesWritten) / float64(job.TotalBytes) * 100
			}
		}
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
		Device     string `json:"device"`     // single device or comma-separated list
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

	cfg := s.config.Get()
	s.config.SetAutoFormat(req.AutoFormat)

	// Split comma-separated devices and deduplicate
	devices := splitDevices(req.Device)
	if len(devices) == 0 {
		writeError(w, http.StatusBadRequest, "no valid devices")
		return
	}

	var started []string
	var conflicts []string

	for _, dev := range devices {
		// Safety check
		if err := device.IsSafeToWipe(dev, cfg.UnsafeAllowAllUSB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Start the wipe
		if err := s.startDeviceWipe(dev, cfg.UnsafeAllowAllUSB); err != nil {
			if _, ok := err.(*jobConflictError); ok {
				conflicts = append(conflicts, dev)
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else {
			started = append(started, dev)
		}
	}

	if len(started) == 0 && len(conflicts) > 0 {
		writeError(w, http.StatusConflict, "all selected devices are already being wiped")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":    "started",
		"started":   started,
		"conflicts": conflicts,
	})
}

// startDeviceWipe begins a wipe on a single device.
func (s *Server) startDeviceWipe(devicePath string, unsafeAllowAllUSB bool) error {
	progress := make(chan wipe.ProgressEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())

	job := &wipe.WipeJob{
		DevicePath: devicePath,
		Status:     wipe.StatusRunning,
		StartedAt:  time.Now(),
	}

	state := &wipeJobState{
		Job:      job,
		progress: progress,
		cancel:   cancel,
	}

	if err := s.jobs.set(devicePath, state); err != nil {
		cancel()
		return err
	}

	// Run wipe in background
	go func() {
		defer close(progress)

		err := wipe.Wipe(ctx, devicePath, progress, unsafeAllowAllUSB)
		now := time.Now()

		if err != nil {
			if ctx.Err() == context.Canceled {
				job.Status = wipe.StatusCancelled
			} else {
				job.Status = wipe.StatusFailed
				job.Error = err.Error()
			}
		} else {
			job.Status = wipe.StatusCompleted

			// Auto-format if requested (uses global config at wipe start time)
			// Note: autoFormat is per-wipe-request, we store it in the job
			// For now, format is controlled by the global config at wipe time
			cfg := s.config.Get()
			if cfg.AutoFormat {
				log.Printf("auto-formatting %s as FAT32", devicePath)
				if formatErr := format.FormatFAT32(devicePath, unsafeAllowAllUSB); formatErr != nil {
					log.Printf("auto-format failed: %v", formatErr)
					job.Status = wipe.StatusFailed
					job.Error = "Wipe completed but format failed: " + formatErr.Error()
				}
			}
		}
		job.FinishedAt = now
	}()

	// Pipe progress to SSE hub
	go func() {
		for ev := range progress {
			// Update job state
			s.jobs.mu.Lock()
			if s.jobs.jobs[devicePath] != nil && s.jobs.jobs[devicePath].Job == job {
				job.TotalBytes = ev.TotalBytes
				job.BytesWritten = ev.BytesWritten
			}
			s.jobs.mu.Unlock()
			s.sseHub.Broadcast(ev)
		}

		// Send final event
		s.sseHub.Broadcast(wipe.ProgressEvent{
			DevicePath:   devicePath,
			Status:       job.Status,
			Message:      "Wipe " + job.Status + " for " + devicePath,
			Timestamp:    time.Now(),
			TotalBytes:   job.TotalBytes,
			BytesWritten: job.BytesWritten,
			Percent:      100,
		})
	}()

	return nil
}

func (s *Server) handlePostCancel(w http.ResponseWriter, r *http.Request) {
	devicePath := r.URL.Query().Get("device")

	if devicePath != "" {
		// Cancel specific device
		state := s.jobs.get(devicePath)
		if state == nil || state.Job.Status != wipe.StatusRunning {
			writeError(w, http.StatusBadRequest, "no active wipe job for "+devicePath)
			return
		}
		state.cancel()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	// Cancel all running jobs
	allJobs := s.jobs.getAll()
	cancelled := 0
	for devicePath, job := range allJobs {
		if job.Status == wipe.StatusRunning {
			state := s.jobs.get(devicePath)
			if state != nil {
				state.cancel()
				cancelled++
			}
		}
	}

	if cancelled == 0 {
		writeError(w, http.StatusBadRequest, "no active wipe jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cancelled": cancelled,
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	devicePath := r.URL.Query().Get("device")

	if devicePath != "" {
		state := s.jobs.get(devicePath)
		if state == nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status": "idle",
				"device": devicePath,
			})
			return
		}
		writeJSON(w, http.StatusOK, state.Job)
		return
	}

	// Return all jobs
	allJobs := s.jobs.getAll()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs": allJobs,
	})
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

	// Send all current job states on connect
	allJobs := s.jobs.getAll()
	for devicePath, job := range allJobs {
		if job.Status == wipe.StatusRunning {
			ev := wipe.ProgressEvent{
				DevicePath:   devicePath,
				BytesWritten: job.BytesWritten,
				TotalBytes:   job.TotalBytes,
				Status:       job.Status,
				Timestamp:    time.Now(),
			}
			if job.TotalBytes > 0 {
				ev.Percent = float64(job.BytesWritten) / float64(job.TotalBytes) * 100
			}
			writeSSEEvent(w, flusher, ev)
		}
	}

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

func splitDevices(input string) []string {
	parts := strings.Split(input, ",")
	seen := make(map[string]bool)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
