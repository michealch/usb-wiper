package notify

import (
	"testing"
)

func TestValidateURL_RejectsLoopback(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/hook",
		"http://127.0.0.1:9999/hook",
		"http://localhost/hook",
		"http://[::1]/hook",
	}
	for _, u := range cases {
		if err := ValidateURL(u); err == nil {
			t.Errorf("expected error for loopback URL %q, got nil", u)
		}
	}
}

func TestValidateURL_RejectsPrivateRFC1918(t *testing.T) {
	cases := []string{
		"http://10.0.0.1/hook",
		"http://192.168.1.1/hook",
		"http://172.16.0.1/hook",
		"http://172.31.255.255/hook",
	}
	for _, u := range cases {
		if err := ValidateURL(u); err == nil {
			t.Errorf("expected error for private URL %q, got nil", u)
		}
	}
}

func TestValidateURL_RejectsLinkLocal(t *testing.T) {
	cases := []string{
		"http://169.254.169.254/latest/meta-data",
		"http://169.254.0.1/hook",
	}
	for _, u := range cases {
		if err := ValidateURL(u); err == nil {
			t.Errorf("expected error for link-local URL %q, got nil", u)
		}
	}
}

func TestValidateURL_RejectsNonHTTP(t *testing.T) {
	cases := []string{
		"ftp://example.com/hook",
		"file:///etc/passwd",
		"gopher://example.com/hook",
	}
	for _, u := range cases {
		if err := ValidateURL(u); err == nil {
			t.Errorf("expected error for non-http URL %q, got nil", u)
		}
	}
}

func TestValidateURL_RejectsInvalid(t *testing.T) {
	if err := ValidateURL("not a url"); err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}
