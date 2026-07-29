package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	in := Store{"SPM": {Token: "t1", State: Written}}
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if out["SPM"].Token != "t1" || out["SPM"].State != Written {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	out, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty store, got %+v", out)
	}
}

func TestLoadCorruptFileIsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Load(p)
	if err != nil {
		t.Fatalf("corrupt file should not error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty store on corrupt file, got %+v", out)
	}
}
