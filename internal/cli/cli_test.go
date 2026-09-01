package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Pin the JSON shape. Integration fixtures only substring-match "Path"; a switch to a non-JSON representation would
// still contain the word "Path" somewhere and slip past them.
func TestPrintVersionIsJSON(t *testing.T) {
	var out bytes.Buffer
	if err := PrintVersion(&out); err != nil {
		t.Fatalf("PrintVersion: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("version output is not valid JSON: %v\ngot: %q", err, out.String())
	}
}
