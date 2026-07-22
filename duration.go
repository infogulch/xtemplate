package xtemplate

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a time.Duration that marshals to/from human-friendly strings
// (e.g. "30s", "1m30s") in JSON and text.
//
// It implements [encoding.TextUnmarshaler] and [encoding.TextMarshaler] so
// go-arg CLI flags parse duration strings the same way.
type Duration time.Duration

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// String formats d using time.Duration.String.
func (d Duration) String() string { return time.Duration(d).String() }

// MarshalText implements [encoding.TextMarshaler].
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler].
func (d *Duration) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*d = 0
		return nil
	}
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", text, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalJSON implements [json.Marshaler]. Zero encodes as "0s".
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements [json.Unmarshaler]. Accepts a duration string
// ("30s") or JSON null.
func (d *Duration) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*d = 0
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("duration must be a string (e.g. \"30s\"): %w", err)
	}
	return d.UnmarshalText([]byte(s))
}
