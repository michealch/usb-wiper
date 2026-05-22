package cert

import (
	"encoding/json"
	"testing"
	"time"
)

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(t.TempDir())
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return s
}

func sampleCert() *Certificate {
	now := time.Now().UTC()
	return NewCertificate("v0.0.0", "test", "/dev/sdb", "TestDisk", "SN123",
		1<<30, "zero", "Zero Fill", 1,
		now, now.Add(time.Second), 1<<30, 1<<20,
		"", "", 0, "skipped")
}

func TestSignAndVerify(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	if err := s.Sign(c); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if c.Version != CertificateVersion {
		t.Fatalf("expected version %d, got %d", CertificateVersion, c.Version)
	}
	if c.Signature == "" {
		t.Fatal("expected non-empty signature")
	}

	data, _ := json.Marshal(c)
	ok, err := s.Verify(data)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("expected valid signature, got false")
	}
}

func TestTamperedCertRejected(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	s.Sign(c)

	data, _ := json.Marshal(c)
	// Flip one byte in the middle of the payload
	mid := len(data) / 2
	data[mid] ^= 0xFF

	// Unmarshal may fail (not valid JSON) or verify may fail — either is correct
	ok, _ := s.Verify(data)
	if ok {
		t.Fatal("tampered certificate should not verify")
	}
}

func TestTamperedFieldRejected(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	s.Sign(c)

	// Marshal, unmarshal, change a field, re-marshal
	data, _ := json.Marshal(c)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	dev := m["device"].(map[string]interface{})
	dev["serial"] = "TAMPERED"
	tampered, _ := json.Marshal(m)

	ok, err := s.Verify(tampered)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if ok {
		t.Fatal("cert with tampered field should not verify")
	}
}

func TestWrongPublicKeyRejected(t *testing.T) {
	s1 := newTestSigner(t)
	s2 := newTestSigner(t)

	c := sampleCert()
	s1.Sign(c)
	data, _ := json.Marshal(c)

	// Verify with s2's public key — should fail
	ok, err := VerifyAgainstPubKey(data, s2.PublicKeyBase64())
	if err != nil {
		t.Fatalf("VerifyAgainstPubKey error: %v", err)
	}
	if ok {
		t.Fatal("wrong public key should not verify")
	}
}

func TestCorrectPublicKeyVerifies(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	s.Sign(c)
	data, _ := json.Marshal(c)

	ok, err := VerifyAgainstPubKey(data, s.PublicKeyBase64())
	if err != nil {
		t.Fatalf("VerifyAgainstPubKey: %v", err)
	}
	if !ok {
		t.Fatal("expected valid signature with correct public key")
	}
}

func TestUnsupportedVersionRejected(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	s.Sign(c)

	data, _ := json.Marshal(c)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	m["version"] = 99
	bad, _ := json.Marshal(m)

	_, err := s.Verify(bad)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestMissingSignatureRejected(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	// Do not sign
	data, _ := json.Marshal(c)

	_, err := s.Verify(data)
	if err == nil {
		t.Fatal("expected error for missing signature, got nil")
	}
}

func TestVersionFieldSetOnSign(t *testing.T) {
	s := newTestSigner(t)
	c := sampleCert()
	c.Version = 0 // ensure it starts at zero
	s.Sign(c)
	if c.Version != CertificateVersion {
		t.Fatalf("Sign must set Version to %d, got %d", CertificateVersion, c.Version)
	}
}
