package logger

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]zerolog.Level{
		"debug":   zerolog.DebugLevel,
		"INFO":    zerolog.InfoLevel,
		"warn":    zerolog.WarnLevel,
		"warning": zerolog.WarnLevel,
		"error":   zerolog.ErrorLevel,
		"":        zerolog.InfoLevel,
		"bogus":   zerolog.InfoLevel,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestInitSetsGlobalLevel(t *testing.T) {
	if _, _, err := Init(Config{Level: "debug", Format: "json", Output: "stdout"}); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("global level = %v, want debug", zerolog.GlobalLevel())
	}
}

func TestWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	global = zerolog.New(&buf).With().Timestamp().Logger()

	withID := WithRequestID("req-123")
	withID.Info().Msg("hello")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid json log: %v (%s)", err, buf.String())
	}
	if entry["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", entry["request_id"])
	}

	// An empty request ID must not add the field.
	buf.Reset()
	noID := WithRequestID("")
	noID.Info().Msg("hello")
	entry = nil
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid json log: %v", err)
	}
	if _, ok := entry["request_id"]; ok {
		t.Error("request_id should be absent for an empty id")
	}
}
