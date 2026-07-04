package command

import (
	"bytes"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stderr bytes.Buffer
	if err := run([]string{"--version"}, "test-version", &stderr); err != nil {
		t.Fatal(err)
	}
	if got := stderr.String(); got != "test-version\n" {
		t.Fatalf("version output = %q, want %q", got, "test-version\n")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{"wat"}, "test-version", &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != `unknown command "wat"` {
		t.Fatalf("error = %q", got)
	}
}
