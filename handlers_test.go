package xtemplate

import "testing"

func TestNegotiateEncoding(t *testing.T) {
	// Common encoding sets used across cases.
	identityOnly := []encodingInfo{{encoding: "identity"}}
	gzipOnly := []encodingInfo{{encoding: "gzip"}}
	identityGzip := []encodingInfo{{encoding: "identity"}, {encoding: "gzip"}}
	full := []encodingInfo{{encoding: "identity"}, {encoding: "gzip"}, {encoding: "br"}}
	noIdentity := []encodingInfo{{encoding: "gzip"}, {encoding: "br"}}

	tests := []struct {
		name         string
		acceptHeader []string
		encodings    []encodingInfo
		wantEncoding string
		wantErr      bool
	}{
		{
			name:         "empty accept headers defaults to identity",
			acceptHeader: nil,
			encodings:    identityGzip,
			wantEncoding: "identity",
		},
		{
			name:         "single identity encoding",
			acceptHeader: []string{"gzip"},
			encodings:    identityOnly,
			wantEncoding: "identity",
		},
		{
			name:         "single non-identity encoding returns it with error",
			acceptHeader: nil,
			encodings:    gzipOnly,
			wantEncoding: "gzip",
			wantErr:      true,
		},
		{
			name:         "accept gzip selects gzip",
			acceptHeader: []string{"gzip"},
			encodings:    full,
			wantEncoding: "gzip",
		},
		{
			name:         "accept gzip or identity prefers identity by ordering",
			acceptHeader: []string{"gzip, identity"},
			encodings:    full,
			wantEncoding: "identity",
		},
		{
			name:         "q within 0.1 band prefers earlier listed identity",
			acceptHeader: []string{"gzip;q=0.5, identity;q=0.41"},
			encodings:    full,
			wantEncoding: "identity",
		},
		{
			name:         "q outside 0.1 band keeps gzip",
			acceptHeader: []string{"gzip;q=0.5, identity;q=0.39"},
			encodings:    full,
			wantEncoding: "gzip",
		},
		{
			name:         "unknown requested encoding falls back to identity",
			acceptHeader: []string{"br"},
			encodings:    identityGzip,
			wantEncoding: "identity",
		},
		{
			name:         "no identity entry returns non-nil encoding with error",
			acceptHeader: nil,
			encodings:    noIdentity,
			wantEncoding: "br",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := negotiateEncoding(tt.acceptHeader, tt.encodings)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got == nil {
				t.Fatalf("expected non-nil encoding, got nil")
			}
			if got.encoding != tt.wantEncoding {
				t.Errorf("encoding = %q, want %q", got.encoding, tt.wantEncoding)
			}
		})
	}
}
