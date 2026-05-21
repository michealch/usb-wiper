package wipe

import (
	"time"
)

// ProgressEvent reports the current state of a wipe operation.
type ProgressEvent struct {
	DevicePath   string        `json:"devicePath"`
	BytesWritten uint64        `json:"bytesWritten"`
	TotalBytes   uint64        `json:"totalBytes"`
	Percent      float64       `json:"percent"`
	Speed        uint64        `json:"speed"` // bytes per second
	ETA          time.Duration `json:"eta"`
	Status       string        `json:"status"`
	Message      string        `json:"message"`
	Timestamp    time.Time     `json:"timestamp"`
}

type speedSample struct {
	bytesWritten uint64
	timestamp    time.Time
}

// computeSpeed calculates a rolling average speed from the last few samples.
func computeSpeed(samples []speedSample) uint64 {
	if len(samples) < 2 {
		return 0
	}

	// Use last 5 samples for a rolling average
	var totalDuration time.Duration
	var totalBytes uint64

	start := 0
	if len(samples) > 5 {
		start = len(samples) - 5
	}

	for i := start + 1; i < len(samples); i++ {
		prev := samples[i-1]
		curr := samples[i]
		duration := curr.timestamp.Sub(prev.timestamp)
		bytes := curr.bytesWritten - prev.bytesWritten

		totalDuration += duration
		totalBytes += bytes
	}

	if totalDuration == 0 {
		return 0
	}

	speed := float64(totalBytes) / totalDuration.Seconds()
	return uint64(speed)
}
