package main

import (
	"strings"
	"testing"
)

func TestHelpTextEmbedded(t *testing.T) {
	if len(helpText) == 0 {
		t.Fatal("helpText is empty; help.txt was not embedded")
	}

	if !strings.Contains(string(helpText), "USAGE:") {
		t.Errorf("helpText missing Usage line")
	}
}
