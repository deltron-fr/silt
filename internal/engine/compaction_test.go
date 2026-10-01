package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCompactionWritesArrayWithoutTrailingSSTable(t *testing.T) {
	t.Chdir(t.TempDir())

	records := make([]KV, 2000)
	for i := range records {
		records[i] = KV{
			Key:   fmt.Sprintf("k%04d", i),
			Value: fmt.Sprintf("v%04d", i),
		}
	}

	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("sst-1.json", data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("MANIFEST", []byte("sst-1.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	e.SSTfileCount = 2
	if err := e.Compaction(); err != nil {
		t.Fatal(err)
	}

	compacted, err := os.ReadFile("sst-2.json")
	if err != nil {
		t.Fatal(err)
	}
	var got []KV
	if err := json.Unmarshal(compacted, &got); err != nil {
		t.Fatalf("compacted SSTable is not a JSON array: %v", err)
	}
	if len(got) != 2000 {
		t.Fatalf("compacted record count = %d, want 2000", len(got))
	}

	if _, err := os.Stat("sst-3.json"); !os.IsNotExist(err) {
		t.Fatalf("sst-3.json exists; want no trailing SSTable")
	}
	manifest, err := os.ReadFile("MANIFEST")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(manifest)) != "sst-2.json" {
		t.Fatalf("manifest = %q, want sst-2.json", strings.TrimSpace(string(manifest)))
	}
}
