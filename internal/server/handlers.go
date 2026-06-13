package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/usb-wiper/internal/audit"
	cert2 "github.com/usb-wiper/internal/cert"
	"github.com/usb-wiper/internal/config"
	"github.com/usb-wiper/internal/device"
	"github.com/usb-wiper/internal/notify"
	"github.com/usb-wiper/internal/persistence"
	"github.com/usb-wiper/internal/presets"
	"github.com/usb-wiper/internal/queue"
	"github.com/usb-wiper/internal/wipe"
)

// ---- Device endpoints ----

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

	// Attach running wipe status from queue
	allJobs := s.jobs.List()
	deviceJobMap := make(map[string]*queue.Job)
	for _, j := range allJobs {
		if j.Status == queue.StatusRunning || j.Status == queue.StatusVerifying || j.Status == queue.StatusFormatting {
			deviceJobMap[j.DevicePath] = j
		}
	}

	for i := range devices {
		if job, ok := deviceJobMap[devices[i].Path]; ok {
			devices[i].Wiping = true
			devices[i].WipeStatus = string(job.Status)
			devices[i].WipePercent = job.Progress
		}

		if latest := s.latestTrustedWipeRecord(devices[i]); latest != nil {
			devices[i].WipeHistory = &device.WipeHistorySummary{
				Status:       latest.Status,
				Verification: latest.Verification,
				FinishedAt:   latest.FinishedAt,
			}
		}
		if s.healthHistory != nil && isTrustedIdentity(devices[i].IdentityConfidence) {
			if latest := s.healthHistory.GetLatestByDeviceID(devices[i].DeviceID); latest != nil {
				devices[i].HealthLatest = &device.HealthSummary{
					HealthStatus:        latest.HealthStatus,
					TemperatureC:        latest.TemperatureC,
					PowerOnHours:        latest.PowerOnHours,
					EnduranceUsedPct:    latest.EnduranceUsedPct,
					UncorrectableErrors: latest.UncorrectableErrors,
					CapturedAt:          latest.CapturedAt,
				}
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

	// Validate device is in the live USB list before shelling out to smartctl
	cfg := s.config.Get()
	devices, _ := device.ListUSBDevices(cfg.UnsafeAllowAllUSB)
	found := false
	var liveDevice *device.Device
	for _, d := range devices {
		if d.Path == devicePath {
			found = true
			copy := d
			liveDevice = &copy
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "device not in detected USB device list")
		return
	}

	health, err := device.GetHealth(devicePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if liveDevice != nil {
		if health.ModelName == "" {
			health.ModelName = liveDevice.Model
		}
		if health.SerialNumber == "" {
			health.SerialNumber = liveDevice.Serial
		}
		if health.FirmwareVersion == "" {
			health.FirmwareVersion = liveDevice.Firmware
		}
		if health.CapacityBytes == 0 {
			health.CapacityBytes = liveDevice.SizeBytes
		}
		s.recordHealthSnapshot(*liveDevice, health)
	}

	writeJSON(w, http.StatusOK, health)
}

func (s *Server) handleGetHealthHistory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "deviceId query parameter required")
		return
	}
	if s.healthHistory == nil {
		writeError(w, http.StatusServiceUnavailable, "health history not available")
		return
	}
	records := s.healthHistory.GetByDeviceID(deviceID)
	if records == nil {
		records = []persistence.HealthRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"history": records,
	})
}

// ---- History endpoints ----

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")
	if deviceID != "" {
		filtered := s.history.GetByDeviceID(deviceID)
		if filtered == nil {
			filtered = []persistence.WipeRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"history": filtered,
		})
		return
	}

	devicePath := r.URL.Query().Get("device")
	if devicePath != "" {
		all := s.history.GetAll()
		var filtered []persistence.WipeRecord
		for _, rec := range all {
			if rec.DevicePath == devicePath {
				filtered = append(filtered, rec)
			}
		}
		if filtered == nil {
			filtered = []persistence.WipeRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"history": filtered,
		})
		return
	}

	all := s.history.GetAll()
	if all == nil {
		all = []persistence.WipeRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"history": all,
	})
}

// ---- Wipe endpoint (queue-based) ----

func (s *Server) handlePostWipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Devices      []string `json:"devices"`
		Device       string   `json:"device"` // legacy single-device
		SchemeID     string   `json:"schemeId"`
		PresetID     string   `json:"presetId"`
		AutoFormat   bool     `json:"autoFormat"`
		VerifySizeGB *int     `json:"verifySizeGB"`
		Label        string   `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Resolve device list
	var devicePaths []string
	if len(req.Devices) > 0 {
		devicePaths = req.Devices
	} else if req.Device != "" {
		devicePaths = splitDevices(req.Device)
	} else {
		writeError(w, http.StatusBadRequest, "devices or device field required")
		return
	}

	// Resolve preset
	cfg := s.config.Get()
	if req.PresetID != "" {
		p, err := s.presets.Get(req.PresetID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "preset not found: "+req.PresetID)
			return
		}
		if req.SchemeID == "" {
			req.SchemeID = p.SchemeID
		}
		if !req.AutoFormat {
			req.AutoFormat = p.AutoFormat
		}
		if req.VerifySizeGB == nil {
			req.VerifySizeGB = &p.VerifySizeGB
		}
		if req.Label == "" && p.LabelTemplate != "" {
			req.Label = resolveLabel(p.LabelTemplate)
		}
	}

	// Default scheme
	if req.SchemeID == "" {
		req.SchemeID = cfg.DefaultSchemeID
	}

	verifySizeGB := cfg.VerifySizeGB
	if req.VerifySizeGB != nil {
		verifySizeGB = *req.VerifySizeGB
	}

	started := []string{}
	conflicts := []string{}

	for _, dev := range devicePaths {
		// Safety check
		if err := device.IsSafeToWipe(dev, cfg.UnsafeAllowAllUSB); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s: %v", dev, err))
			return
		}

		liveDevice := s.findLiveDevice(dev)
		_, err := s.jobs.Enqueue(enqueueRequestForDevice(dev, liveDevice, req.SchemeID, req.AutoFormat, verifySizeGB, req.Label))
		if err != nil {
			if errors.Is(err, queue.ErrJobAlreadyActive) {
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
		writeError(w, http.StatusConflict, "all selected devices are already being wiped or queued")
		return
	}

	s.config.SetAutoFormat(req.AutoFormat)
	s.sendRefreshEvent()

	if len(started) > 0 {
		s.auditEvent(r, "wipe_started", strings.Join(started, ","), map[string]interface{}{
			"scheme":     req.SchemeID,
			"autoFormat": req.AutoFormat,
			"verify_gb":  verifySizeGB,
		})
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":    "started",
		"started":   started,
		"conflicts": conflicts,
	})
}

// ---- Cancel endpoint ----

func (s *Server) handlePostCancel(w http.ResponseWriter, r *http.Request) {
	devicePath := r.URL.Query().Get("device")

	if devicePath != "" {
		if err := s.jobs.CancelDevice(devicePath); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.auditEvent(r, "wipe_cancelled", devicePath, nil)
		s.sendRefreshEvent()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	cancelled := s.jobs.CancelAll()
	if cancelled == 0 {
		writeError(w, http.StatusBadRequest, "no active or queued wipe jobs")
		return
	}

	s.auditEvent(r, "wipe_cancelled_all", "", map[string]interface{}{"count": cancelled})
	s.sendRefreshEvent()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cancelled": cancelled,
	})
}

// ---- Test wipe endpoint ----

func (s *Server) handlePostTestWipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device       string `json:"device"`
		VerifySizeGB *int   `json:"verifySizeGB"`
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

	if err := device.IsSafeToWipe(req.Device, cfg.UnsafeAllowAllUSB); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	verifySizeGB := cfg.VerifySizeGB
	if req.VerifySizeGB != nil {
		verifySizeGB = *req.VerifySizeGB
	}

	liveDevice := s.findLiveDevice(req.Device)
	_, err := s.jobs.Enqueue(enqueueRequestForDevice(req.Device, liveDevice, "zero", false, verifySizeGB, "test-wipe"))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	s.sendRefreshEvent()
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "started",
		"device": req.Device,
	})
}

// ---- Job endpoints ----

func (s *Server) handleGetJobs(w http.ResponseWriter, r *http.Request) {
	jobs := s.jobs.List()
	if jobs == nil {
		jobs = []*queue.Job{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs": jobs,
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.jobs.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handlePostCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.jobs.Cancel(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.sendRefreshEvent()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- Scheme endpoints ----

func (s *Server) handleGetSchemes(w http.ResponseWriter, r *http.Request) {
	schemes := s.schemes.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schemes": schemes,
	})
}

// ---- Preset endpoints ----

func (s *Server) handleGetPresets(w http.ResponseWriter, r *http.Request) {
	all := s.presets.List()
	if all == nil {
		all = []presets.Preset{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"presets": all,
	})
}

func (s *Server) handlePostPresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		SchemeID      string `json:"schemeId"`
		AutoFormat    bool   `json:"autoFormat"`
		VerifySizeGB  int    `json:"verifySizeGB"`
		LabelTemplate string `json:"labelTemplate"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if req.SchemeID == "" {
		req.SchemeID = "zero"
	}

	p, err := s.presets.Create(req.Name, req.SchemeID, req.AutoFormat, req.VerifySizeGB, req.LabelTemplate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePutPreset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Name          *string `json:"name"`
		SchemeID      *string `json:"schemeId"`
		AutoFormat    *bool   `json:"autoFormat"`
		VerifySizeGB  *int    `json:"verifySizeGB"`
		LabelTemplate *string `json:"labelTemplate"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	p, err := s.presets.Update(id, req.Name, req.SchemeID, req.AutoFormat, req.VerifySizeGB, req.LabelTemplate)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePreset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.presets.Delete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- Settings endpoint ----

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.config.Get())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var updates config.ConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate webhook URL if being set
	if updates.WebhookURL != nil && *updates.WebhookURL != "" {
		if err := notify.ValidateURL(*updates.WebhookURL); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid webhook URL: %v", err))
			return
		}
	}

	if updates.AutoWipeEnabled != nil && *updates.AutoWipeEnabled && !s.config.Get().AutoWipeEnabled {
		if s.autoWipe == nil {
			writeError(w, http.StatusServiceUnavailable, "auto wipe state store not available")
			return
		}
		if _, err := s.markCurrentDevicesSeen("observed_on_enable", "Auto wipe enabled; connected devices marked seen"); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	s.config.Update(updates)
	writeJSON(w, http.StatusOK, s.config.Get())
}

// ---- Backward-compat config endpoints ----

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.config.Get())
}

func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	var cfg struct {
		AutoFormat   bool `json:"autoFormat"`
		VerifySizeGB *int `json:"verifySizeGB"`
	}

	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	s.config.SetAutoFormat(cfg.AutoFormat)
	if cfg.VerifySizeGB != nil {
		s.config.SetVerifySizeGB(*cfg.VerifySizeGB)
	}

	writeJSON(w, http.StatusOK, s.config.Get())
}

// ---- SSE endpoint ----

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

	// Send keepalive heartbeat every 25s to keep proxy connections alive
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	// Subscribe before replaying missed events so we don't miss events
	// that arrive in the gap between replay and subscribe.
	ch := s.sseHub.Subscribe()
	defer s.sseHub.Unsubscribe(ch)

	// Replay missed events if the client sent Last-Event-ID.
	if lastIDStr := r.Header.Get("Last-Event-ID"); lastIDStr != "" {
		var lastID uint64
		fmt.Sscanf(lastIDStr, "%d", &lastID)
		if lastID > 0 {
			for _, entry := range s.sseHub.EventsSince(lastID) {
				writeSSEEventID(w, flusher, entry.ev, entry.id)
			}
		}
	} else {
		// Fresh connection — send current running job states so the UI
		// can show in-progress wipes without waiting for the next event.
		allJobs := s.jobs.List()
		for _, job := range allJobs {
			if job.Status == queue.StatusRunning || job.Status == queue.StatusVerifying || job.Status == queue.StatusFormatting {
				ev := wipe.ProgressEvent{
					EventType:   "job",
					DevicePath:  job.DevicePath,
					Status:      string(job.Status),
					Percent:     job.Progress,
					CurrentPass: job.CurrentPass,
					TotalPasses: job.TotalPasses,
					Timestamp:   time.Now(),
				}
				writeSSEEvent(w, flusher, ev)
			}
		}
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			// SSE comment line — keeps the connection alive through proxies
			w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(w, flusher, ev)
		}
	}
}

// ---- Health check ----

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// ---- SSE event writer for named events ----

func init() {
	// Ensure wipe package is loaded
	_ = wipe.StatusRunning
}

// ---- Helpers ----

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

func resolveLabel(template string) string {
	now := time.Now()
	result := template
	result = strings.ReplaceAll(result, "{date}", now.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "{datetime}", now.Format("2006-01-02T15:04:05"))
	return result
}

func isTrustedIdentity(confidence string) bool {
	return confidence == "high" || confidence == "medium"
}

func (s *Server) latestTrustedWipeRecord(d device.Device) *persistence.WipeRecord {
	if !isTrustedIdentity(d.IdentityConfidence) {
		return nil
	}
	return s.history.GetLatestByDeviceID(d.DeviceID)
}

func (s *Server) findLiveDevice(devicePath string) *device.Device {
	cfg := s.config.Get()
	devices, err := device.ListUSBDevices(cfg.UnsafeAllowAllUSB)
	if err != nil {
		return nil
	}
	for _, d := range devices {
		if d.Path == devicePath {
			copy := d
			return &copy
		}
	}
	return nil
}

func enqueueRequestForDevice(devicePath string, d *device.Device, schemeID string, autoFormat bool, verifySizeGB int, label string) queue.EnqueueRequest {
	req := queue.EnqueueRequest{
		DevicePath:   devicePath,
		SchemeID:     schemeID,
		AutoFormat:   autoFormat,
		VerifySizeGB: verifySizeGB,
		Label:        label,
	}
	if d != nil {
		req.DeviceID = d.DeviceID
		req.IdentitySource = d.IdentitySource
		req.IdentityConfidence = d.IdentityConfidence
		req.DeviceModel = d.Model
		req.DeviceSerial = d.Serial
		req.DeviceFirmware = d.Firmware
		req.DeviceWWN = d.WWN
		req.DeviceSizeBytes = d.SizeBytes
	}
	return req
}

func defaultSchemeID(cfg config.Config) string {
	if strings.TrimSpace(cfg.DefaultSchemeID) == "" {
		return "zero"
	}
	return cfg.DefaultSchemeID
}

func isAutoWipeCandidate(d device.Device) bool {
	return d.DeviceID != "" && strings.TrimSpace(d.Serial) != "" && isTrustedIdentity(d.IdentityConfidence)
}

func (s *Server) markCurrentDevicesSeen(action, message string) (int, error) {
	cfg := s.config.Get()
	devices, err := device.ListUSBDevices(cfg.UnsafeAllowAllUSB)
	if err != nil {
		return 0, err
	}
	return s.markDevicesSeen(devices, action, message)
}

func (s *Server) markDevicesSeen(devices []device.Device, action, message string) (int, error) {
	if s.autoWipe == nil {
		return 0, nil
	}
	marked := 0
	for _, d := range devices {
		if !isAutoWipeCandidate(d) {
			continue
		}
		if s.autoWipe.Has(d.DeviceID) {
			continue
		}
		rec := autoWipeRecordForDevice(d, action, "", message)
		if err := s.autoWipe.Upsert(rec); err != nil {
			return marked, err
		}
		marked++
	}
	return marked, nil
}

func (s *Server) handleAutoWipeDevices(devices []device.Device, cfg config.Config) {
	if s.autoWipe == nil || !cfg.AutoWipeEnabled {
		return
	}

	schemeID := defaultSchemeID(cfg)
	for _, d := range devices {
		if !isAutoWipeCandidate(d) || s.autoWipe.Has(d.DeviceID) {
			continue
		}

		if d.WipeBlocked {
			s.recordAutoWipeDecision(d, "skipped", "", d.BlockReason)
			continue
		}

		if err := device.IsSafeToWipe(d.Path, cfg.UnsafeAllowAllUSB); err != nil {
			s.recordAutoWipeDecision(d, "skipped", "", err.Error())
			continue
		}

		job, err := s.jobs.Enqueue(enqueueRequestForDevice(d.Path, &d, schemeID, cfg.AutoFormat, cfg.VerifySizeGB, "auto-wipe"))
		if err != nil {
			action := "error"
			if errors.Is(err, queue.ErrJobAlreadyActive) {
				action = "conflict"
			}
			s.recordAutoWipeDecision(d, action, "", err.Error())
			continue
		}

		msg := fmt.Sprintf("Queued default scheme %s for new serial %s", schemeID, d.Serial)
		s.recordAutoWipeDecision(d, "queued", job.ID, msg)
		if s.auditLog != nil {
			s.auditLog.Log(audit.Event{
				Event:  "auto_wipe_queued",
				Actor:  "system",
				Target: d.Path,
				Details: map[string]interface{}{
					"job_id":              job.ID,
					"scheme":              schemeID,
					"device_id":           d.DeviceID,
					"serial":              d.Serial,
					"identity_source":     d.IdentitySource,
					"identity_confidence": d.IdentityConfidence,
				},
			})
		}
		s.sendRefreshEvent()
	}
}

func (s *Server) recordAutoWipeDecision(d device.Device, action, jobID, message string) {
	if s.autoWipe == nil {
		return
	}
	if err := s.autoWipe.Upsert(autoWipeRecordForDevice(d, action, jobID, message)); err != nil {
		log.Printf("WARNING: failed to persist auto-wipe state for %s: %v", d.Path, err)
	}
}

func autoWipeRecordForDevice(d device.Device, action, jobID, message string) persistence.AutoWipeRecord {
	return persistence.AutoWipeRecord{
		DeviceID:           d.DeviceID,
		Serial:             d.Serial,
		Model:              d.Model,
		IdentitySource:     d.IdentitySource,
		IdentityConfidence: d.IdentityConfidence,
		LastSeenAt:         time.Now(),
		LastDevicePath:     d.Path,
		LastAction:         action,
		LastJobID:          jobID,
		LastMessage:        message,
	}
}

func (s *Server) recordHealthSnapshot(d device.Device, h *device.Health) {
	if s.healthHistory == nil || h == nil {
		return
	}
	rec := persistence.HealthRecord{
		DeviceID:            d.DeviceID,
		DevicePath:          d.Path,
		IdentitySource:      d.IdentitySource,
		IdentityConfidence:  d.IdentityConfidence,
		Model:               firstNonEmpty(h.ModelName, d.Model),
		Serial:              firstNonEmpty(h.SerialNumber, d.Serial),
		Firmware:            firstNonEmpty(h.FirmwareVersion, d.Firmware),
		WWN:                 d.WWN,
		SizeBytes:           firstNonZero(h.CapacityBytes, d.SizeBytes),
		CapturedAt:          time.Now(),
		HealthStatus:        h.HealthStatus,
		DeviceType:          h.DeviceType,
		PowerOnHours:        h.PowerOnHours,
		PowerCycleCount:     h.PowerCycleCount,
		TemperatureC:        h.TemperatureC,
		ReadLBAs:            h.ReadLBAs,
		WriteLBAs:           h.WriteLBAs,
		AvailableSparePct:   h.AvailableSparePct,
		EnduranceUsedPct:    h.EnduranceUsedPct,
		ReallocatedSectors:  h.ReallocatedSectors,
		PendingSectors:      h.PendingSectors,
		UncorrectableErrors: h.UncorrectableErrors,
		Raw:                 h.Raw,
	}
	if err := s.healthHistory.Append(rec); err != nil {
		log.Printf("WARNING: failed to persist health snapshot for %s: %v", d.Path, err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZero(values ...uint64) uint64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

// ---- Certificate endpoints ----

func (s *Server) handleGetPubKey(w http.ResponseWriter, r *http.Request) {
	if s.signer == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate signing not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"publicKey": s.signer.PublicKeyBase64(),
	})
}

func (s *Server) handleGetCertJSON(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	// Find job in queue (job IDs are ULIDs stored in the queue)
	job, err := s.jobs.Get(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	if job.Status != queue.StatusCompleted && job.Status != queue.StatusFailed {
		writeError(w, http.StatusBadRequest, "certificate only available for completed/failed jobs")
		return
	}

	cert := s.buildCertificate(job)
	if s.signer != nil {
		if err := s.signer.Sign(cert); err != nil {
			log.Printf("WARNING: failed to sign certificate: %v", err)
		}
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"certificate-%s.json\"", jobID))
	writeJSON(w, http.StatusOK, cert)
}

func (s *Server) handleGetCertPDF(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	job, err := s.jobs.Get(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	cert := s.buildCertificate(job)
	if s.signer != nil {
		s.signer.Sign(cert)
	}

	pdf := cert2.GeneratePDF(cert)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"certificate-%s.pdf\"", jobID))
	w.Write(pdf)
}

func (s *Server) handlePostCertVerify(w http.ResponseWriter, r *http.Request) {
	if s.signer == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate verification not available")
		return
	}

	// Cap body at 1 MiB to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	certData, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read request body: "+err.Error())
		return
	}

	valid, err := s.signer.Verify(certData)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid": valid,
	})
}

func (s *Server) buildCertificate(job *queue.Job) *cert2.Certificate {
	model := job.DeviceModel
	serial := job.DeviceSerial
	size := job.DeviceSizeBytes
	if model == "" || serial == "" || size == 0 {
		devInfo, _ := device.GetDevice(job.DevicePath)
		if devInfo != nil {
			model = firstNonEmpty(model, devInfo.Model)
			serial = firstNonEmpty(serial, devInfo.Serial)
			size = firstNonZero(size, devInfo.SizeBytes)
		}
	}

	scheme, _ := s.schemes.Get(job.SchemeID)
	schemeName := job.SchemeID
	schemePasses := job.TotalPasses
	if scheme != nil {
		schemeName = scheme.DisplayName()
		schemePasses = scheme.Passes()
	}

	startedAt := time.Now()
	if job.StartedAt != nil {
		startedAt = *job.StartedAt
	}
	completedAt := time.Now()
	if job.CompletedAt != nil {
		completedAt = *job.CompletedAt
	}

	verifyResult := "skipped"
	if job.Verified == "passed" {
		verifyResult = "passed"
	} else if job.Verified == "failed" {
		verifyResult = "failed"
	}

	return cert2.NewCertificate(
		"dev", "unknown",
		job.DevicePath, model, serial, size,
		job.SchemeID, schemeName, schemePasses,
		startedAt, completedAt,
		0, 0,
		"", "",
		job.BytesVerified, verifyResult,
	)
}

// ---- Audit endpoint ----

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeError(w, http.StatusServiceUnavailable, "audit log not available")
		return
	}

	events, err := s.auditLog.Read(200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []audit.Event{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
	})
}

// ---- Auto wipe endpoints ----

func (s *Server) handleGetAutoWipe(w http.ResponseWriter, r *http.Request) {
	cfg := s.config.Get()
	records := []persistence.AutoWipeRecord{}
	if s.autoWipe != nil {
		records = s.autoWipe.List()
	}

	schemeID := defaultSchemeID(cfg)
	schemeName := schemeID
	if scheme, err := s.schemes.Get(schemeID); err == nil {
		schemeName = scheme.DisplayName()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available":         s.autoWipe != nil,
		"enabled":           cfg.AutoWipeEnabled && s.autoWipe != nil,
		"configuredEnabled": cfg.AutoWipeEnabled,
		"defaultSchemeId":   schemeID,
		"defaultSchemeName": schemeName,
		"autoFormat":        cfg.AutoFormat,
		"verifySizeGB":      cfg.VerifySizeGB,
		"seen":              records,
	})
}

func (s *Server) handlePutAutoWipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled field required")
		return
	}
	if s.autoWipe == nil {
		writeError(w, http.StatusServiceUnavailable, "auto wipe state store not available")
		return
	}

	prev := s.config.Get().AutoWipeEnabled
	marked := 0
	if *req.Enabled && !prev {
		var err error
		marked, err = s.markCurrentDevicesSeen("observed_on_enable", "Auto wipe enabled; connected devices marked seen")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	s.config.Update(config.ConfigUpdate{AutoWipeEnabled: req.Enabled})
	s.auditEvent(r, "auto_wipe_configured", "", map[string]interface{}{
		"enabled":          *req.Enabled,
		"connected_marked": marked,
	})

	cfg := s.config.Get()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":         cfg.AutoWipeEnabled,
		"connectedMarked": marked,
		"seen":            s.autoWipe.List(),
	})
}

func (s *Server) handleDeleteAutoWipeSeen(w http.ResponseWriter, r *http.Request) {
	if s.autoWipe == nil {
		writeError(w, http.StatusServiceUnavailable, "auto wipe state store not available")
		return
	}
	if err := s.autoWipe.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditEvent(r, "auto_wipe_seen_cleared", "", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
