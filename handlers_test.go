package xtemplate

import "testing"

// encs builds an encodingInfo slice from coding names, in the given order. The
// order matters: negotiateEncoding breaks q-value ties toward earlier entries,
// and the builder sorts real encodings size-ascending.
func encs(names ...string) []encodingInfo {
	out := make([]encodingInfo, len(names))
	for i, n := range names {
		out[i] = encodingInfo{encoding: n}
	}
	return out
}

func TestNegotiateEncoding(t *testing.T) {
	// For a tiny file, gzip overhead makes the compressed copy larger than the
	// original, so identity sorts first.
	tiny := encs("identity", "gzip")
	// For a real CSS file, br < gzip < identity by size.
	css := encs("br", "gzip", "identity")

	tests := []struct {
		name    string
		encs    []encodingInfo
		accept  []string
		want    string // expected encoding; "" with wantNil means 406
		wantNil bool
	}{
		// Behaviors locked in by test/tests/assets.hurl.
		{"no header serves identity", tiny, nil, "identity", false},
		{"empty header serves identity", tiny, []string{""}, "identity", false},
		{"gzip requested", tiny, []string{"gzip"}, "gzip", false},
		{"gzip or identity prefers identity (earlier in list)", tiny, []string{"gzip, identity"}, "identity", false},
		{"0.09 gzip pref still identity (within tie threshold)", tiny, []string{"gzip;q=0.5, identity;q=0.41"}, "identity", false},
		{"0.11 gzip pref selects gzip (beyond threshold)", tiny, []string{"gzip;q=0.5, identity;q=0.39"}, "gzip", false},
		{"unavailable coding falls back to identity", tiny, []string{"br"}, "identity", false},
		{"css gzip", css, []string{"gzip"}, "gzip", false},
		{"css gzip or br prefers br (earlier/smaller)", css, []string{"gzip, br"}, "br", false},

		// New: q=0 means "not acceptable" (RFC 7231 §5.3.4).
		{"identity refused selects available gzip", tiny, []string{"gzip, identity;q=0"}, "gzip", false},
		{"identity refused with no alternative is 406", tiny, []string{"identity;q=0"}, "", true},
		{"gzip refused falls back to identity", tiny, []string{"gzip;q=0"}, "identity", false},

		// New: wildcard support.
		{"wildcard accepts smallest available", css, []string{"*"}, "br", false},
		{"wildcard q=0 refuses everything", tiny, []string{"*;q=0"}, "", true},
		{"explicit overrides wildcard refusal", tiny, []string{"*;q=0, gzip"}, "gzip", false},
		{"wildcard refuses identity but names gzip", tiny, []string{"*;q=0, gzip;q=1"}, "gzip", false},
		{"identity explicit survives wildcard q=0", tiny, []string{"*;q=0, identity"}, "identity", false},

		// Multiple header lines are combined.
		{"split across header lines", tiny, []string{"gzip;q=0.5", "identity;q=0.39"}, "gzip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := negotiateEncoding(tt.accept, tt.encs)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil (406) result, got %q (err=%v)", got.encoding, err)
				}
				if err == nil {
					t.Error("expected a non-nil error explaining why nothing is acceptable")
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil result, want %q (err=%v)", tt.want, err)
			}
			if got.encoding != tt.want {
				t.Errorf("encoding = %q, want %q", got.encoding, tt.want)
			}
		})
	}
}

func TestNegotiateEncoding_NoEncodings(t *testing.T) {
	got, err := negotiateEncoding(nil, nil)
	if got != nil {
		t.Errorf("expected nil result for empty encodings, got %v", got)
	}
	if err == nil {
		t.Error("expected an error for empty encodings")
	}
}
