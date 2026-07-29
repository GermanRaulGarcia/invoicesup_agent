package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValidWithPollDefault(t *testing.T) {
	p := writeTemp(t, `{"base_url":"https://x","token":"tok","folder":"/tmp/out"}`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.BaseURL != "https://x" || c.Token != "tok" || c.Folder != "/tmp/out" {
		t.Fatalf("unexpected config: %+v", c)
	}
	if c.PollSeconds != 30 {
		t.Fatalf("expected poll default 30, got %d", c.PollSeconds)
	}
}

func TestLoadRejectsMissingFields(t *testing.T) {
	cases := map[string]string{
		"no base_url": `{"token":"t","folder":"/o"}`,
		"no token":    `{"base_url":"https://x","folder":"/o"}`,
		"no folder":   `{"base_url":"https://x","token":"t"}`,
		"poll too low": `{"base_url":"https://x","token":"t","folder":"/o","poll_seconds":2}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeTemp(t, body)); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestLoadMissingFileErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
