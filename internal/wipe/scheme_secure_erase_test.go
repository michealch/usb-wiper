package wipe

import "testing"

func TestParseATASecurity(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		wantSupported bool
		wantFrozen    bool
		wantEnhanced  bool
	}{
		{
			name: "typical healthy drive is supported and not frozen",
			output: `ATA device, with non-removable media
	Model Number:       Example Drive
	Security: 
		Master password revision code = 65534
			supported
		not	enabled
		not	locked
		not	frozen
		not	expired: security count
			supported: enhanced erase
	2min for SECURITY ERASE UNIT. 2min for ENHANCED SECURITY ERASE UNIT.
Checksum: correct`,
			wantSupported: true,
			wantFrozen:    false,
			wantEnhanced:  true,
		},
		{
			name: "frozen drive",
			output: `	Security: 
		Master password revision code = 65534
			supported
			enabled
			locked
			frozen
		not	expired: security count`,
			wantSupported: true,
			wantFrozen:    true,
		},
		{
			name: "unsupported drive",
			output: `	Security: 
		Master password revision code = 65534
		not	supported`,
			wantSupported: false,
		},
		{
			name: "no security section",
			output: `ATA device, with non-removable media
	Model Number:       Example Drive`,
			wantSupported: false,
			wantFrozen:    false,
		},
		{
			name: "case-insensitive enhanced erase",
			output: `	Security: 
			supported
			supported: Enhanced Erase
		2min for SECURITY ERASE UNIT. 2min for ENHANCED SECURITY ERASE UNIT.`,
			wantSupported: true,
			wantEnhanced:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported, frozen, enhanced := parseATASecurity(tt.output)
			if supported != tt.wantSupported {
				t.Errorf("supported = %v, want %v", supported, tt.wantSupported)
			}
			if frozen != tt.wantFrozen {
				t.Errorf("frozen = %v, want %v", frozen, tt.wantFrozen)
			}
			if enhanced != tt.wantEnhanced {
				t.Errorf("enhanced = %v, want %v", enhanced, tt.wantEnhanced)
			}
		})
	}
}
