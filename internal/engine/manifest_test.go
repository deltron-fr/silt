package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestStartupWithEmptyManifest(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "MANIFEST")
	if err := os.WriteFile(manifest, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	e.Manifest = manifest
	if err := e.manifestStartup(); err != nil {
		t.Fatal(err)
	}
	if e.SSTfileCount != 0 {
		t.Fatalf("SSTfileCount = %d, want 0", e.SSTfileCount)
	}
}

func TestManifestStartupDerivesNextIDFromSingleEntry(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "MANIFEST")
	if err := os.WriteFile(manifest, []byte("sst-1.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	e.Manifest = manifest
	if err := e.manifestStartup(); err != nil {
		t.Fatal(err)
	}
	if e.SSTfileCount != 2 {
		t.Fatalf("SSTfileCount = %d, want 2", e.SSTfileCount)
	}
}

func TestManifestStartupDerivesNextIDFromLatestEntry(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "MANIFEST")
	contents := "sst-1.json\nsst-2.json\nsst-3.json\n"
	if err := os.WriteFile(manifest, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	e.Manifest = manifest
	if err := e.manifestStartup(); err != nil {
		t.Fatal(err)
	}
	if e.SSTfileCount != 4 {
		t.Fatalf("SSTfileCount = %d, want 4", e.SSTfileCount)
	}
}
