package xtemplate

import (
	"reflect"
	"testing"
)

func TestExtractFrontMatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMeta map[string]any
		wantBody string
		wantErr  bool
	}{
		{
			name:     "yaml fence closed with dashes",
			input:    "---\ntitle: Hello\n---\nbody content",
			wantMeta: map[string]any{"title": "Hello"},
			wantBody: "\nbody content",
		},
		{
			name:     "yaml fence closed with dots",
			input:    "---\ntitle: Hello\n...\nbody content",
			wantMeta: map[string]any{"title": "Hello"},
			wantBody: "\nbody content",
		},
		{
			name:     "toml fence",
			input:    "+++\ntitle = \"Hi\"\n+++\nbody content",
			wantMeta: map[string]any{"title": "Hi"},
			wantBody: "\nbody content",
		},
		{
			name:     "json fence",
			input:    "{\n\"title\": \"J\"\n}\nbody content",
			wantMeta: map[string]any{"title": "J"},
			wantBody: "\nbody content",
		},
		{
			name:     "no front matter returns whole body and nil meta",
			input:    "hello world\nmore body",
			wantMeta: nil,
			wantBody: "hello world\nmore body",
		},
		{
			name:     "leading blank lines before fence",
			input:    "\n\n---\ntitle: X\n---\nbody content",
			wantMeta: map[string]any{"title": "X"},
			wantBody: "\nbody content",
		},
		{
			name:     "crlf line endings",
			input:    "---\r\ntitle: CR\r\n---\r\nbody content",
			wantMeta: map[string]any{"title": "CR"},
			wantBody: "\r\nbody content",
		},
		{
			name:    "unterminated fence returns error",
			input:   "---\ntitle: X\nbody with no closing fence",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body, err := extractFrontMatter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil (meta=%v body=%q)", meta, body)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(meta, tt.wantMeta) {
				t.Errorf("meta = %#v, want %#v", meta, tt.wantMeta)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
