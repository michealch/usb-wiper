//go:build linux

package queue

import (
	"context"
	"testing"
	"time"

	"github.com/usb-wiper/internal/persistence"
	"github.com/usb-wiper/internal/wipe"
)

// nullSSEHub discards all broadcasts.
type nullSSEHub struct{}

func (n *nullSSEHub) Broadcast(wipe.ProgressEvent) {}

// fakeScheme is a Scheme that completes immediately without touching any device.
type fakeScheme struct{ id string }

func (f *fakeScheme) ID() string          { return f.id }
func (f *fakeScheme) DisplayName() string { return f.id }
func (f *fakeScheme) Passes() int         { return 1 }
func (f *fakeScheme) Execute(_ context.Context, _ string, _ uint64, progress chan<- wipe.ProgressEvent) error {
	progress <- wipe.ProgressEvent{Percent: 100, CurrentPass: 1, TotalPasses: 1}
	return nil
}

func newTestQueue(t *testing.T) *Queue {
	t.Helper()
	dir := t.TempDir()
	hist, err := persistence.New(dir)
	if err != nil {
		t.Fatalf("persistence.New: %v", err)
	}
	reg := wipe.NewSchemeRegistry()
	return New(Config{
		MaxParallel: 2,
		SSEHub:      &nullSSEHub{},
		History:     hist,
		Schemes:     reg,
		UnsafeAllow: true,
	})
}

func TestEnqueueAndGet(t *testing.T) {
	q := newTestQueue(t)
	job, err := q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "zero"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
	got, err := q.Get(job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != job.ID {
		t.Fatalf("expected job ID %q, got %q", job.ID, got.ID)
	}
}

func TestDuplicateDeviceRejected(t *testing.T) {
	q := newTestQueue(t)
	_, err := q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "zero"})
	if err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	_, err = q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "zero"})
	if err == nil {
		t.Fatal("expected error for duplicate device enqueue, got nil")
	}
}

func TestCancelQueuedJob(t *testing.T) {
	q := newTestQueue(t)
	job, _ := q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "zero"})

	if err := q.Cancel(job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got, _ := q.Get(job.ID)
	if got.Status != StatusCancelled {
		t.Fatalf("expected status %q, got %q", StatusCancelled, got.Status)
	}
}

func TestCancelNonexistentJobErrors(t *testing.T) {
	q := newTestQueue(t)
	if err := q.Cancel("does-not-exist"); err == nil {
		t.Fatal("expected error cancelling unknown job ID")
	}
}

func TestUnknownSchemeRejected(t *testing.T) {
	q := newTestQueue(t)
	_, err := q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "totally-fake"})
	if err == nil {
		t.Fatal("expected error for unknown scheme, got nil")
	}
}

func TestSafetyCheckFailsForNonexistentDevice(t *testing.T) {
	q := newTestQueue(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go q.Start(ctx)

	// Enqueue a job for a device path that cannot pass IsSafeToWipe
	job, err := q.Enqueue(EnqueueRequest{DevicePath: "/dev/nonexistent-device-xzy", SchemeID: "zero"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := q.Get(job.ID)
		if got.Status == StatusFailed || got.Status == StatusCancelled {
			return // expected: safety check blocked the wipe
		}
		time.Sleep(25 * time.Millisecond)
	}
	got, _ := q.Get(job.ID)
	t.Fatalf("expected job to fail due to safety check, got status %q (err: %q)", got.Status, got.ErrorMessage)
}

func TestListReturnsAllJobs(t *testing.T) {
	q := newTestQueue(t)
	q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "zero"})
	q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdc", SchemeID: "zero"})
	jobs := q.List()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestCancelAllCancelsQueued(t *testing.T) {
	q := newTestQueue(t)
	q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdb", SchemeID: "zero"})
	q.Enqueue(EnqueueRequest{DevicePath: "/dev/sdc", SchemeID: "zero"})

	n := q.CancelAll()
	if n != 2 {
		t.Fatalf("expected 2 cancelled, got %d", n)
	}
	for _, j := range q.List() {
		if j.Status != StatusCancelled {
			t.Errorf("expected cancelled, got %q for job %s", j.Status, j.ID)
		}
	}
}
