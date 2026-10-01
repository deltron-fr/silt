package engine

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func useTempEngineDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func TestWALRecordIncludesEmptyValue(t *testing.T) {
	record, err := json.Marshal(WALRecord{Op: OpPut, Key: "k", Value: ""})
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(record, &fields); err != nil {
		t.Fatal(err)
	}

	value, ok := fields["value"]
	if !ok {
		t.Fatalf("WAL record does not contain a value field: %s", record)
	}
	if string(value) != `""` {
		t.Fatalf("value field = %s, want empty string", value)
	}
}

func TestStartupCreatesMissingWAL(t *testing.T) {
	useTempEngineDir(t)

	if err := NewEngine().StartUp(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat("wal.db")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("new WAL size = %d, want 0", info.Size())
	}
}

func TestWALAppendsNDJSONRecords(t *testing.T) {
	useTempEngineDir(t)
	e := NewEngine()
	if err := e.StartUp(); err != nil {
		t.Fatal(err)
	}

	if err := e.UpsertKeyValue("k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := e.UpsertKeyValue("k", "v2"); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open("wal.db")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var records []WALRecord
	for scanner.Scan() {
		var record WALRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	if len(records) != 2 {
		t.Fatalf("WAL records = %d, want 2", len(records))
	}
	if records[0].Value != "v1" || records[1].Value != "v2" {
		t.Fatalf("WAL values = %q, %q; want v1, v2", records[0].Value, records[1].Value)
	}
}

func TestStartupReplaysWALInOrder(t *testing.T) {
	useTempEngineDir(t)
	first := NewEngine()
	if err := first.StartUp(); err != nil {
		t.Fatal(err)
	}
	if err := first.UpsertKeyValue("updated", "old"); err != nil {
		t.Fatal(err)
	}
	if err := first.UpsertKeyValue("updated", "new"); err != nil {
		t.Fatal(err)
	}
	if err := first.UpsertKeyValue("empty", ""); err != nil {
		t.Fatal(err)
	}

	second := NewEngine()
	if err := second.StartUp(); err != nil {
		t.Fatal(err)
	}

	if got, err := second.RetrieveKeyValue("updated"); err != nil || got != "new" {
		t.Fatalf("replayed updated value = %q, %v; want new", got, err)
	}
	if got, err := second.RetrieveKeyValue("empty"); err != nil || got != "" {
		t.Fatalf("replayed empty value = %q, %v; want empty string", got, err)
	}
}

func TestFlushResetsWALAndRestartUsesNextSSTableID(t *testing.T) {
	useTempEngineDir(t)
	first := NewEngine()
	if err := first.StartUp(); err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 2000; i++ {
		key := "k" + strconv.Itoa(i)
		if err := first.UpsertKeyValue(key, "v"+strconv.Itoa(i)); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}

	walInfo, err := os.Stat("wal.db")
	if err != nil {
		t.Fatal(err)
	}
	if walInfo.Size() != 0 {
		t.Fatalf("WAL size after flush = %d, want 0", walInfo.Size())
	}
	if _, err := os.Stat("sst-1.json"); err != nil {
		t.Fatal(err)
	}

	second := NewEngine()
	if err := second.StartUp(); err != nil {
		t.Fatal(err)
	}
	if second.SSTfileCount != 2 {
		t.Fatalf("next SSTable ID = %d, want 2", second.SSTfileCount)
	}
	if got, err := second.RetrieveKeyValue("k1"); err != nil || got != "v1" {
		t.Fatalf("flushed value = %q, %v; want v1", got, err)
	}

	for i := 2001; i <= 4000; i++ {
		key := "k" + strconv.Itoa(i)
		if err := second.UpsertKeyValue(key, "v"+strconv.Itoa(i)); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}

	if _, err := os.Stat(filepath.Join(".", "sst-2.json")); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("MANIFEST")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(manifest)); got != "sst-1.json\nsst-2.json" {
		t.Fatalf("manifest = %q", got)
	}
}
