// Package notify provides webhook-based completion notifications.
// Webhook URLs are validated against SSRF — private IPs and redirects are
// blocked unless UNSAFE_ALLOW_PRIVATE_WEBHOOKS=1 is set.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// WebhookPayload is sent to a webhook URL on job completion.
type WebhookPayload struct {
	Event      string    `json:"event"`
	JobID      string    `json:"jobId"`
	DevicePath string    `json:"devicePath"`
	Status     string    `json:"status"`
	SchemeID   string    `json:"schemeId"`
	Label      string    `json:"label,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// ErrUnsafeWebhook is returned when a webhook URL targets a private/internal address.
var ErrUnsafeWebhook = fmt.Errorf("webhook URL resolves to a private/internal address; set UNSAFE_ALLOW_PRIVATE_WEBHOOKS=1 to allow")

// Notifier sends webhook notifications.
type Notifier struct {
	webhookURL     string
	client         *http.Client
	allowPrivateIP bool
}

// New creates a new notifier. The webhook URL is validated; if it resolves
// to a loopback, link-local, or RFC 1918 address, the notifier is created
// but will reject sends unless unsafeAllowPrivate is true.
func New(webhookURL string) *Notifier {
	if webhookURL == "" {
		return &Notifier{}
	}

	allowPrivate := os.Getenv("UNSAFE_ALLOW_PRIVATE_WEBHOOKS") == "1"

	return &Notifier{
		webhookURL:     webhookURL,
		allowPrivateIP: allowPrivate,
		client: &http.Client{
			Timeout: 10 * time.Second,
			// Block redirects to prevent SSRF through open redirectors
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ValidateURL checks whether a webhook URL is safe to use.
// Returns ErrUnsafeWebhook if the URL targets a private, loopback, or
// link-local address and allowPrivate is not set.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL scheme must be http or https, got %q", u.Scheme)
	}

	// Allow via env opt-in
	if os.Getenv("UNSAFE_ALLOW_PRIVATE_WEBHOOKS") == "1" {
		return nil
	}

	return validateHost(u.Host)
}

// validateHost checks that a host:port resolves to a public IP address.
func validateHost(host string) error {
	// Strip port for resolution
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("cannot resolve webhook host %q: %w", hostname, err)
	}

	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("%w: %s resolves to %s", ErrUnsafeWebhook, hostname, ip.String())
		}
	}
	return nil
}

// isPublicIP returns true if the IP is not loopback, link-local, private, or multicast.
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	return true
}

// SendJobComplete sends a notification for a completed job.
// Returns ErrUnsafeWebhook if the target is private and private IPs are not allowed.
func (n *Notifier) SendJobComplete(jobID, devicePath, status, schemeID, label string) error {
	if n.webhookURL == "" {
		return nil
	}

	// Re-validate at send time
	if err := ValidateURL(n.webhookURL); err != nil {
		return err
	}

	payload := WebhookPayload{
		Event:      "wipe_complete",
		JobID:      jobID,
		DevicePath: devicePath,
		Status:     status,
		SchemeID:   schemeID,
		Label:      label,
		Timestamp:  time.Now(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook: %w", err)
	}

	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("webhook post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
