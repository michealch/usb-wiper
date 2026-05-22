// Package cert provides tamper-evident certificates of erasure with Ed25519 signing.
package cert

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CertificateVersion is the current signing format version.
// v1: legacy (signed SHA-256 hash of canonical JSON) — no longer emitted.
// v2: signs canonical JSON directly (standard RFC 8032 Ed25519).
const CertificateVersion = 2

// Certificate is a tamper-evident record of a completed wipe operation.
type Certificate struct {
	Version     int              `json:"version"`
	Tool        ToolInfo         `json:"tool"`
	Host        HostInfo         `json:"host"`
	Device      DeviceInfo       `json:"device"`
	Wipe        WipeInfo         `json:"wipe"`
	Verification VerificationInfo `json:"verification"`
	Signature   string           `json:"signature,omitempty"`
}

type ToolInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
}

type HostInfo struct {
	Hostname string `json:"hostname"`
	Kernel   string `json:"kernel"`
}

type DeviceInfo struct {
	Model    string `json:"model"`
	Serial   string `json:"serial"`
	Size     string `json:"size"`
	SysfsPath string `json:"sysfsPath"`
}

type WipeInfo struct {
	SchemeID    string `json:"schemeId"`
	SchemeName  string `json:"schemeName"`
	Passes      int    `json:"passes"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
	Duration    string `json:"duration"`
	BytesWritten string `json:"bytesWritten"`
	AvgSpeed    string `json:"avgSpeed"`
	PreHash     string `json:"preHash,omitempty"`
	PostHash    string `json:"postHash,omitempty"`
}

type VerificationInfo struct {
	BytesVerified string `json:"bytesVerified"`
	Result        string `json:"result"` // "passed", "failed", "skipped"
}

// Signer manages Ed25519 key pair for certificate signing.
type Signer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyDir     string
}

// NewSigner loads or generates an Ed25519 key pair (PEM-encoded PKCS#8).
func NewSigner(dataDir string) (*Signer, error) {
	keyDir := filepath.Join(dataDir, "keys")
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("create keys dir: %w", err)
	}

	privPath := filepath.Join(keyDir, "signing.ed25519")
	pubPath := filepath.Join(keyDir, "signing.ed25519.pub")

	var priv ed25519.PrivateKey
	var pub ed25519.PublicKey

	if data, err := os.ReadFile(privPath); err == nil {
		// Try PEM first (new format), then raw bytes (legacy)
		priv = tryLoadPrivateKey(data)
		if priv == nil {
			return nil, fmt.Errorf("unrecognized private key format in %s", privPath)
		}
	} else if os.IsNotExist(err) {
		// Generate new key pair
		pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		priv = priv2
		pub = pub2

		// Write private key as PEM-encoded PKCS#8 (0600)
		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, fmt.Errorf("marshal private key: %w", err)
		}
		privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
		if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
			return nil, fmt.Errorf("write private key: %w", err)
		}

		// Write public key as PEM-encoded SubjectPublicKeyInfo (0644)
		pubBytes, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			return nil, fmt.Errorf("marshal public key: %w", err)
		}
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
		if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
			return nil, fmt.Errorf("write public key: %w", err)
		}
	} else {
		return nil, fmt.Errorf("read key: %w", err)
	}

	// Load public key
	if pub == nil {
		data, err := os.ReadFile(pubPath)
		if err != nil {
			return nil, fmt.Errorf("read public key: %w", err)
		}
		pub = tryLoadPublicKey(data)
		if pub == nil {
			return nil, fmt.Errorf("unrecognized public key format in %s", pubPath)
		}
	}

	return &Signer{
		privateKey: priv,
		publicKey:  pub,
		keyDir:     keyDir,
	}, nil
}

// tryLoadPrivateKey attempts to load a PEM-encoded PKCS#8 key or legacy raw bytes.
func tryLoadPrivateKey(data []byte) ed25519.PrivateKey {
	// Try PEM
	block, _ := pem.Decode(data)
	if block != nil {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			if edKey, ok := key.(ed25519.PrivateKey); ok {
				return edKey
			}
		}
	}
	// Legacy: raw 64-byte seed
	if len(data) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(data)
	}
	return nil
}

// tryLoadPublicKey attempts to load a PEM-encoded SubjectPublicKeyInfo or legacy raw bytes.
func tryLoadPublicKey(data []byte) ed25519.PublicKey {
	// Try PEM
	block, _ := pem.Decode(data)
	if block != nil {
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err == nil {
			if edKey, ok := key.(ed25519.PublicKey); ok {
				return edKey
			}
		}
	}
	// Legacy: raw 32-byte public key
	if len(data) == ed25519.PublicKeySize {
		return ed25519.PublicKey(data)
	}
	return nil
}

// PublicKeyPEM returns the public key in PEM format (base64).
func (s *Signer) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(s.publicKey)
}

// Sign generates an Ed25519 signature over the canonical JSON of the certificate.
// The signature field is excluded (set to empty) before signing.
// Ed25519 already hashes internally (SHA-512 per RFC 8032); we sign the
// canonical JSON directly, which is interoperable with any Ed25519 verifier.
func (s *Signer) Sign(cert *Certificate) error {
	cert.Version = CertificateVersion
	cert.Signature = ""
	canonical, err := canonicalJSON(cert)
	if err != nil {
		return err
	}

	sig := ed25519.Sign(s.privateKey, canonical)
	cert.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}

// Verify checks the certificate signature against the public key.
func (s *Signer) Verify(certData []byte) (bool, error) {
	var cert Certificate
	if err := json.Unmarshal(certData, &cert); err != nil {
		return false, fmt.Errorf("parse certificate: %w", err)
	}

	sig := cert.Signature
	if sig == "" {
		return false, fmt.Errorf("no signature in certificate")
	}
	if cert.Version != 0 && cert.Version != CertificateVersion {
		return false, fmt.Errorf("unsupported certificate version %d (this build only verifies v%d)", cert.Version, CertificateVersion)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false, fmt.Errorf("decode signature: %w", err)
	}

	cert.Signature = ""
	canonical, err := canonicalJSON(&cert)
	if err != nil {
		return false, err
	}

	return ed25519.Verify(s.publicKey, canonical, sigBytes), nil
}

// VerifyAgainstPubKey verifies a certificate against a given public key.
func VerifyAgainstPubKey(certData []byte, pubKeyBase64 string) (bool, error) {
	pubBytes, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		return false, fmt.Errorf("decode pubkey: %w", err)
	}
	pub := ed25519.PublicKey(pubBytes)

	var cert Certificate
	if err := json.Unmarshal(certData, &cert); err != nil {
		return false, fmt.Errorf("parse: %w", err)
	}
	if cert.Version != 0 && cert.Version != CertificateVersion {
		return false, fmt.Errorf("unsupported certificate version %d (this build only verifies v%d)", cert.Version, CertificateVersion)
	}

	sig := cert.Signature
	cert.Signature = ""
	canonical, err := canonicalJSON(&cert)
	if err != nil {
		return false, err
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false, fmt.Errorf("decode sig: %w", err)
	}

	return ed25519.Verify(pub, canonical, sigBytes), nil
}

// canonicalJSON produces a stable JSON representation for signing.
// It ensures keys are sorted and whitespace is minimized.
func canonicalJSON(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Compact the JSON to remove whitespace
	var tmp interface{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return nil, err
	}
	return json.Marshal(tmp)
}

// NewCertificate creates a certificate from job and host info.
func NewCertificate(toolVersion, toolBuildTime, devicePath, model, serial string,
	size uint64, schemeID, schemeName string, passes int,
	startedAt, completedAt time.Time, bytesWritten, avgSpeedBytes uint64,
	preHash, postHash string, bytesVerified uint64, verifyResult string) *Certificate {

	hostname, _ := os.Hostname()
	kernel := readKernel()

	return &Certificate{
		Tool: ToolInfo{
			Name:      "USB Wiper",
			Version:   toolVersion,
			BuildTime: toolBuildTime,
		},
		Host: HostInfo{
			Hostname: hostname,
			Kernel:   kernel,
		},
		Device: DeviceInfo{
			Model:     model,
			Serial:    serial,
			Size:      fmt.Sprintf("%d", size),
			SysfsPath: devicePath,
		},
		Wipe: WipeInfo{
			SchemeID:    schemeID,
			SchemeName:  schemeName,
			Passes:      passes,
			StartedAt:   startedAt.UTC().Format(time.RFC3339),
			CompletedAt: completedAt.UTC().Format(time.RFC3339),
			Duration:    completedAt.Sub(startedAt).Round(time.Second).String(),
			BytesWritten: fmt.Sprintf("%d", bytesWritten),
			AvgSpeed:    fmt.Sprintf("%d B/s", avgSpeedBytes),
			PreHash:     preHash,
			PostHash:    postHash,
		},
		Verification: VerificationInfo{
			BytesVerified: fmt.Sprintf("%d", bytesVerified),
			Result:        verifyResult,
		},
	}
}

func readKernel() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.Split(string(data), " (")[0])
}
