// Package queue provides a FIFO job queue for wipe operations with
// configurable concurrency and persistence integration.
package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/usb-wiper/internal/device"
	"github.com/usb-wiper/internal/persistence"
	"github.com/usb-wiper/internal/ulid"
	"github.com/usb-wiper/internal/wipe"
)

// JobStatus represents the lifecycle state of a wipe job.
type JobStatus string

const (
	StatusQueued     JobStatus = "queued"
	StatusRunning    JobStatus = "running"
	StatusVerifying  JobStatus = "verifying"
	StatusFormatting JobStatus = "formatting"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
	StatusCancelled  JobStatus = "cancelled"
)

// Job represents a single wipe operation in the queue.
type Job struct {
	ID            string     `json:"id"`
	DevicePath    string     `json:"devicePath"`
	SchemeID      string     `json:"schemeId"`
	AutoFormat    bool       `json:"autoFormat"`
	VerifySizeGB  int        `json:"verifySizeGB"`
	Label         string     `json:"label,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	Status        JobStatus  `json:"status"`
	Progress      float64    `json:"progress"`
	CurrentPass   int        `json:"currentPass"`
	TotalPasses   int        `json:"totalPasses"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
	Verified      string     `json:"verified,omitempty"`   // "passed", "failed"
	BytesVerified uint64     `json:"bytesVerified"`
	cancel        context.CancelFunc
}

// ProgressEvent sent via the SSE channel when queue state changes.
type ProgressEvent struct {
	EventType string `json:"eventType"` // "job"
	Job       *Job   `json:"job"`
}

// SSEHub is the interface the queue needs from the SSE layer.
type SSEHub interface {
	Broadcast(wipe.ProgressEvent)
}

// Queue manages wipe jobs with FIFO ordering and concurrency control.
type Queue struct {
	mu          sync.Mutex
	jobs        map[string]*Job // by ULID
	byDevice    map[string]*Job // currently active per device
	pending     []*Job          // FIFO wait list
	maxParallel int
	sem         chan struct{}
	sseHub      SSEHub
	persist     *persistence.Store
	history     *persistence.Store
	schemes     *wipe.SchemeRegistry
	unsafe      bool
	dispatchCh  chan struct{} // signals dispatcher to check for work
	parentCtx   context.Context
}

// Config holds initialization parameters for the queue.
type Config struct {
	MaxParallel int
	SSEHub      SSEHub
	History     *persistence.Store
	Schemes     *wipe.SchemeRegistry
	UnsafeAllow bool
}

// New creates a new wipe job queue.
func New(cfg Config) *Queue {
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 2
	}
	q := &Queue{
		jobs:        make(map[string]*Job),
		byDevice:    make(map[string]*Job),
		pending:     make([]*Job, 0),
		maxParallel: cfg.MaxParallel,
		sem:         make(chan struct{}, cfg.MaxParallel),
		sseHub:      cfg.SSEHub,
		history:     cfg.History,
		schemes:     cfg.Schemes,
		unsafe:      cfg.UnsafeAllow,
		dispatchCh:  make(chan struct{}, 64),
	}
	return q
}

// Start begins the dispatch loop. Should be called in a goroutine.
func (q *Queue) Start(ctx context.Context) {
	q.parentCtx = ctx
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.dispatchCh:
			q.dispatch()
		}
	}
}

// EnqueueRequest contains all parameters needed to create a wipe job.
type EnqueueRequest struct {
	DevicePath   string `json:"devicePath"`
	SchemeID     string `json:"schemeId"`
	AutoFormat   bool   `json:"autoFormat"`
	VerifySizeGB int    `json:"verifySizeGB"`
	Label        string `json:"label,omitempty"`
}

// ErrJobAlreadyActive is returned when attempting to enqueue a job for
// a device that already has an active or queued job.
var ErrJobAlreadyActive = fmt.Errorf("device already has an active or queued job")

// Enqueue creates a new wipe job and adds it to the pending queue.
// Returns an error if the device is already in the queue or running.
func (q *Queue) Enqueue(req EnqueueRequest) (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check device already active
	if _, ok := q.byDevice[req.DevicePath]; ok {
		return nil, ErrJobAlreadyActive
	}

	// Check pending
	for _, j := range q.pending {
		if j.DevicePath == req.DevicePath {
			return nil, ErrJobAlreadyActive
		}
	}

	// Validate scheme
	scheme, err := q.schemes.Get(req.SchemeID)
	if err != nil {
		return nil, err
	}

	job := &Job{
		ID:           ulid.New(),
		DevicePath:   req.DevicePath,
		SchemeID:     req.SchemeID,
		AutoFormat:   req.AutoFormat,
		VerifySizeGB: req.VerifySizeGB,
		Label:        req.Label,
		CreatedAt:    time.Now(),
		Status:       StatusQueued,
		TotalPasses:  scheme.Passes(),
	}

	q.jobs[job.ID] = job
	q.pending = append(q.pending, job)

	// Signal dispatcher
	select {
	case q.dispatchCh <- struct{}{}:
	default:
	}

	// Use broadcastJobLocked since caller holds q.mu
	q.broadcastJobLocked(job)
	return job, nil
}

// Cancel cancels a job by ID. Works on queued, running, and verifying jobs.
func (q *Queue) Cancel(jobID string) error {
	q.mu.Lock()
	job, ok := q.jobs[jobID]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("job %s not found", jobID)
	}

	if job.Status == StatusCompleted || job.Status == StatusFailed || job.Status == StatusCancelled {
		q.mu.Unlock()
		return fmt.Errorf("job %s is already %s", jobID, job.Status)
	}

	// If queued, remove from pending
	if job.Status == StatusQueued {
		q.removePending(job)
	}
	q.mu.Unlock()

	// Cancel via context if running
	if job.cancel != nil {
		job.cancel()
	}

	q.mu.Lock()
	if job.Status != StatusCompleted && job.Status != StatusFailed {
		now := time.Now()
		job.Status = StatusCancelled
		job.CompletedAt = &now
		delete(q.byDevice, job.DevicePath)
	}
	q.mu.Unlock()

	q.broadcastJob(job)
	return nil
}

// CancelDevice cancels the current job for a specific device path.
func (q *Queue) CancelDevice(devicePath string) error {
	q.mu.Lock()
	job, ok := q.byDevice[devicePath]
	if !ok {
		// Check pending
		for _, j := range q.pending {
			if j.DevicePath == devicePath {
				job = j
				break
			}
		}
	}
	if job == nil {
		q.mu.Unlock()
		return fmt.Errorf("no active job for device %s", devicePath)
	}
	q.mu.Unlock()
	return q.Cancel(job.ID)
}

// CancelAll cancels all active and pending jobs.
func (q *Queue) CancelAll() int {
	q.mu.Lock()
	var toCancel []*Job
	for _, j := range q.jobs {
		if j.Status == StatusQueued || j.Status == StatusRunning || j.Status == StatusVerifying || j.Status == StatusFormatting {
			toCancel = append(toCancel, j)
		}
	}
	// Remove all pending
	q.pending = q.pending[:0]
	q.mu.Unlock()

	for _, j := range toCancel {
		if j.cancel != nil {
			j.cancel()
		}
	}

	q.mu.Lock()
	now := time.Now()
	for _, j := range toCancel {
		if j.Status != StatusCompleted && j.Status != StatusFailed {
			j.Status = StatusCancelled
			j.CompletedAt = &now
			delete(q.byDevice, j.DevicePath)
		}
		q.broadcastJobLocked(j)
	}
	q.mu.Unlock()

	return len(toCancel)
}

// List returns all jobs (active and completed).
func (q *Queue) List() []*Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]*Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		out = append(out, j)
	}
	return out
}

// Get returns a job by ID.
func (q *Queue) Get(id string) (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}
	return j, nil
}

// ActiveCount returns the number of currently running jobs.
func (q *Queue) ActiveCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.byDevice)
}

// PendingCount returns the number of queued jobs.
func (q *Queue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// ---- Internal dispatch ----

func (q *Queue) dispatch() {
	q.mu.Lock()
	if len(q.pending) == 0 || len(q.byDevice) >= q.maxParallel {
		q.mu.Unlock()
		return
	}
	// Pop first pending
	job := q.pending[0]
	q.pending = q.pending[1:]
	q.byDevice[job.DevicePath] = job
	q.mu.Unlock()

	// Start job in background
	go q.runJob(job)
}

func (q *Queue) removePending(job *Job) {
	for i, j := range q.pending {
		if j.ID == job.ID {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return
		}
	}
}

func (q *Queue) runJob(job *Job) {
	// Acquire concurrency semaphore
	q.sem <- struct{}{}
	defer func() { <-q.sem }()

	scheme, err := q.schemes.Get(job.SchemeID)
	if err != nil {
		q.failJob(job, "unknown scheme: "+err.Error())
		return
	}

	ctx, cancel := context.WithCancel(q.parentCtx)
	q.mu.Lock()
	job.cancel = cancel
	now := time.Now()
	job.StartedAt = &now
	job.Status = StatusRunning
	job.TotalPasses = scheme.Passes()
	q.mu.Unlock()

	// Create progress channel
	progress := make(chan wipe.ProgressEvent, 64)
	done := make(chan error, 1)

	// ---- CRITICAL: Re-validate safety before destructive operation ----
	// Per AGENTS.md non-negotiable #2: "Always call IsSafeToWipe before every
	// destructive operation." The device could have changed between enqueue and now.
	if err := device.IsSafeToWipe(job.DevicePath, q.unsafe); err != nil {
		msg := fmt.Sprintf("safety re-check failed: %v", err)
		q.failJob(job, msg)
		return
	}

	// Run the scheme
	go func() {
		devSize, sizeErr := getDeviceSize(job.DevicePath)
		if sizeErr != nil {
			done <- fmt.Errorf("get device size: %w", sizeErr)
			return
		}
		done <- scheme.Execute(ctx, job.DevicePath, devSize, progress)
	}()

	// Pipe progress to SSE and update job state
	go func() {
		for ev := range progress {
			q.mu.Lock()
			// Update job fields with progress data during any active phase
			if job.Status == StatusRunning || job.Status == StatusVerifying || job.Status == StatusFormatting {
				job.Progress = ev.Percent
				job.CurrentPass = ev.CurrentPass
				if job.TotalPasses == 0 {
					job.TotalPasses = ev.TotalPasses
				}
			}
			q.mu.Unlock()

			// Forward via SSE hub
			q.sseHub.Broadcast(ev)
		}
	}()

	// Wait for completion
	runErr := <-done
	close(progress)

	now = time.Now()

	if runErr != nil {
		q.mu.Lock()
		if ctx.Err() == context.Canceled {
			job.Status = StatusCancelled
		} else {
			job.Status = StatusFailed
			job.ErrorMessage = runErr.Error()
		}
		job.CompletedAt = &now
		delete(q.byDevice, job.DevicePath)
		q.mu.Unlock()
		writeHistory(q.history, job)
		q.broadcastJob(job)
		q.signalDispatch()
		return
	}

	// ---- Verification phase ----
	if job.VerifySizeGB > 0 {
		// Broadcast status change to "verifying"
		q.mu.Lock()
		job.Status = StatusVerifying
		q.mu.Unlock()
		q.broadcastJob(job)

		// Re-validate safety before reading device for verification
		if err := device.IsSafeToWipe(job.DevicePath, q.unsafe); err != nil {
			q.mu.Lock()
			job.Status = StatusFailed
			job.ErrorMessage = "Safety re-check before verification failed: " + err.Error()
			job.CompletedAt = &now
			delete(q.byDevice, job.DevicePath)
			q.mu.Unlock()
			writeHistory(q.history, job)
			q.broadcastJob(job)
			q.signalDispatch()
			return
		}

		devSize, sizeErr := getDeviceSize(job.DevicePath)
		if sizeErr != nil {
			q.mu.Lock()
			job.Status = StatusFailed
			job.ErrorMessage = "Cannot determine device size for verification: " + sizeErr.Error()
			job.CompletedAt = &now
			delete(q.byDevice, job.DevicePath)
			q.mu.Unlock()
			writeHistory(q.history, job)
			q.broadcastJob(job)
			q.signalDispatch()
			return
		}
		verifySize := uint64(job.VerifySizeGB) * 1024 * 1024 * 1024

		// Use a dedicated verification progress channel so the frontend
		// sees live verification progress.
		vProgress := make(chan wipe.ProgressEvent, 64)
		go func() {
			for ev := range vProgress {
				q.mu.Lock()
				if job.Status == StatusVerifying {
					job.Progress = ev.Percent
					job.BytesVerified = ev.BytesWritten
				}
				q.mu.Unlock()
				q.sseHub.Broadcast(ev)
			}
		}()

		vBytes, vErr := wipe.VerifyRandomChunks(job.DevicePath, devSize, verifySize, vProgress)
		close(vProgress)
		if vErr != nil {
			log.Printf("verification failed for %s: %v", job.DevicePath, vErr)
			q.mu.Lock()
			job.Status = StatusFailed
			job.ErrorMessage = "Verification failed: " + vErr.Error()
			job.Verified = "failed"
			job.CompletedAt = &now
			delete(q.byDevice, job.DevicePath)
			q.mu.Unlock()
			writeHistory(q.history, job)
			q.broadcastJob(job)
			q.signalDispatch()
			return
		}
		job.BytesVerified = vBytes
		job.Verified = "passed"
	}

	// ---- Auto-format ----
	if job.AutoFormat {
		q.mu.Lock()
		job.Status = StatusFormatting
		q.mu.Unlock()
		q.broadcastJob(job)

		// Re-validate safety before formatting (defense in depth; format.FormatFAT32
		// also checks, but the queue worker should not rely on the callee).
		if err := device.IsSafeToWipe(job.DevicePath, q.unsafe); err != nil {
			q.mu.Lock()
			job.Status = StatusFailed
			job.ErrorMessage = "Safety re-check before format failed: " + err.Error()
			job.CompletedAt = &now
			delete(q.byDevice, job.DevicePath)
			q.mu.Unlock()
			writeHistory(q.history, job)
			q.broadcastJob(job)
			q.signalDispatch()
			return
		}

		// Inline format call — format package dependency
		if fErr := formatDevice(job.DevicePath, q.unsafe); fErr != nil {
			log.Printf("format failed for %s: %v", job.DevicePath, fErr)
			q.mu.Lock()
			job.Status = StatusFailed
			job.ErrorMessage = "Wipe completed but format failed: " + fErr.Error()
			job.CompletedAt = &now
			delete(q.byDevice, job.DevicePath)
			q.mu.Unlock()
			writeHistory(q.history, job)
			q.broadcastJob(job)
			q.signalDispatch()
			return
		}
	}

	// ---- Success ----
	q.mu.Lock()
	job.Status = StatusCompleted
	job.Progress = 100
	job.CompletedAt = &now
	delete(q.byDevice, job.DevicePath)
	q.mu.Unlock()

	writeHistory(q.history, job)
	q.broadcastJob(job)

	// Send final wipe progress event via SSE
	q.sseHub.Broadcast(wipe.ProgressEvent{
		EventType:     "refresh",
		DevicePath:    job.DevicePath,
		Status:        "completed",
		BytesVerified: job.BytesVerified,
		Verified:      job.Verified,
		Timestamp:     time.Now(),
	})

	q.signalDispatch()
}

func (q *Queue) failJob(job *Job, msg string) {
	q.mu.Lock()
	job.Status = StatusFailed
	job.ErrorMessage = msg
	now := time.Now()
	job.CompletedAt = &now
	delete(q.byDevice, job.DevicePath)
	q.mu.Unlock()
	writeHistory(q.history, job)
	q.broadcastJob(job)
	q.signalDispatch()
}

func (q *Queue) signalDispatch() {
	select {
	case q.dispatchCh <- struct{}{}:
	default:
	}
}

func (q *Queue) broadcastJob(job *Job) {
	// Take a snapshot under lock, then broadcast without holding the lock.
	q.mu.Lock()
	ev := wipe.ProgressEvent{
		EventType:  "job",
		DevicePath: job.DevicePath,
		Status:     string(job.Status),
		Percent:    job.Progress,
		CurrentPass: job.CurrentPass,
		TotalPasses: job.TotalPasses,
		BytesWritten: job.BytesVerified,
		Timestamp:  time.Now(),
	}
	q.mu.Unlock()

	q.sseHub.Broadcast(ev)
}

func (q *Queue) broadcastJobLocked(job *Job) {
	// Same as broadcastJob but caller already holds q.mu.
	q.sseHub.Broadcast(wipe.ProgressEvent{
		EventType:   "job",
		DevicePath:  job.DevicePath,
		Status:      string(job.Status),
		Percent:     job.Progress,
		CurrentPass: job.CurrentPass,
		TotalPasses: job.TotalPasses,
		BytesWritten: job.BytesVerified,
		Timestamp:   time.Now(),
	})
}

// getDeviceSize opens the device and queries its size via ioctl.
func getDeviceSize(devicePath string) (uint64, error) {
	f, err := openDevice(devicePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return blkGetSize64(f)
}

// openDevice opens a device for reading (to query size).
func openDevice(devicePath string) (*osFd, error) {
	f, err := openOSFile(devicePath)
	if err != nil {
		return nil, err
	}
	return &osFd{f}, nil
}
