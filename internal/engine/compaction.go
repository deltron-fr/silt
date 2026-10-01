package engine

import (
	"bufio"
	"container/heap"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Iterator struct {
	Records []KV
	pos     int
}

type heapItem struct {
	record KV
	itrIdx int
}

func (it *Iterator) Peek() (KV, bool) {
	if it.pos >= len(it.Records) {
		return KV{}, false
	}

	return it.Records[it.pos], true
}

func (it *Iterator) Next() { it.pos++ }

type minHeap []heapItem

func (h minHeap) Len() int { return len(h) }
func (h minHeap) Less(i, j int) bool {
	if h[i].record.Key < h[j].record.Key {
		return true
	} else if h[i].record.Key > h[j].record.Key {
		return false
	}

	// MANIFEST lists SSTables oldest to newest, so the larger iterator index
	// must win ties; otherwise an older value could overwrite a newer one.
	return h[i].itrIdx > h[j].itrIdx
}
func (h minHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)   { *h = append(*h, x.(heapItem)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (e *Engine) Compaction() error {
	manifestFile, err := os.Open(e.Manifest)
	if err != nil {
		return err
	}
	defer manifestFile.Close()

	// Compaction currently reads every SSTable in the MANIFEST. That is what makes it
	// safe to discard a tombstone.
	iters := make([]*Iterator, 0, 100)
	scanner := bufio.NewScanner(manifestFile)
	oldSSTFiles := make([]string, 0, 20)

	for scanner.Scan() {
		fileName := scanner.Text()
		oldSSTFiles = append(oldSSTFiles, fileName)
		records, err := parseSSTRecords(fileName)
		if err != nil {
			return err
		}

		iters = append(iters, &Iterator{Records: records})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	h := &minHeap{}

	for i, iter := range iters {
		if rec, ok := iter.Peek(); ok {
			heap.Push(h, heapItem{record: rec, itrIdx: i})
		}
	}

	newSSTFiles := make([]string, 0, 10)
	fileCount := e.SSTfileCount
	fileName := fmt.Sprintf("sst-%d.json", fileCount)
	var newSSTFile *os.File

	recordCounter := 0
	firstRecord := true

	for h.Len() > 0 {
		newestItem := heap.Pop(h).(heapItem)
		// The heap gives us the newest version first for this key. Advance every
		// older duplicate now so none of them can re-enter the merge later.
		for h.Len() > 0 && (*h)[0].record.Key == newestItem.record.Key {
			dup := heap.Pop(h).(heapItem)
			it := iters[dup.itrIdx]
			it.Next()
			if rec, ok := it.Peek(); ok {
				heap.Push(h, heapItem{record: rec, itrIdx: dup.itrIdx})
			}
		}

		if newestItem.record.Deleted {
			it := iters[newestItem.itrIdx]
			it.Next()
			if rec, ok := it.Peek(); ok {
				heap.Push(h, heapItem{record: rec, itrIdx: newestItem.itrIdx})
			}
			continue
		}

		if recordCounter >= 2000 {
			// Do not open the next file at the boundary. Opening it here would
			// leave an empty SSTable when the current file is the final one.
			if err := closeCompactionSSTable(newSSTFile, fileName); err != nil {
				return fmt.Errorf("closing compacted SSTable: %w", err)
			}
			newSSTFile = nil
			fileCount++
			fileName = fmt.Sprintf("sst-%d.json", fileCount)
			recordCounter = 0
			firstRecord = true
		}

		if newSSTFile == nil {
			newSSTFile, err = os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("opening compacted SSTable: %w", err)
			}
			newSSTFiles = append(newSSTFiles, fileName)
		}

		recordCounter++
		if err := e.writeToSSTableCompaction(newSSTFile, fileName, newestItem.record.Key, newestItem.record.Value, firstRecord); err != nil {
			closeErr := closeCompactionSSTable(newSSTFile, fileName)
			if closeErr != nil {
				return fmt.Errorf("writing compacted SSTable: %w; closing it: %v", err, closeErr)
			}
			return fmt.Errorf("writing compacted SSTable: %w", err)
		}
		firstRecord = false

		it := iters[newestItem.itrIdx]
		it.Next()
		if rec, ok := it.Peek(); ok {
			heap.Push(h, heapItem{record: rec, itrIdx: newestItem.itrIdx})
		}
	}

	if newSSTFile != nil {
		if err := closeCompactionSSTable(newSSTFile, fileName); err != nil {
			return fmt.Errorf("closing compacted SSTable: %w", err)
		}
	}

	manifestData := []byte(strings.Join(newSSTFiles, "\n"))
	if len(manifestData) > 0 {
		manifestData = append(manifestData, '\n')
	}
	// Publish the new MANIFEST before deleting old files. A crash in between
	// leaves extra files, but never leaves the manifest pointing at missing data.
	if err := e.updateManifest(manifestData); err != nil {
		return fmt.Errorf("updating manifest after compaction: %w", err)
	}
	if len(newSSTFiles) > 0 {
		e.SSTfileCount = fileCount + 1
	}

	if err := e.deleteOldFiles(oldSSTFiles); err != nil {
		return fmt.Errorf("deleting old SSTables: %w", err)
	}

	return nil
}

func (e *Engine) writeToSSTableCompaction(sstFile *os.File, filename, key, value string, firstRecord bool) error {
	fileWriter := bufio.NewWriter(sstFile)
	// Records are streamed one at a time, so the caller supplies the delimiter
	// state needed to keep the on-disk representation a valid JSON array.
	if firstRecord {
		if err := fileWriter.WriteByte('['); err != nil {
			return err
		}
	} else if err := fileWriter.WriteByte(','); err != nil {
		return err
	}

	js, err := json.Marshal(KV{Key: key, Value: value})
	if err != nil {
		return fmt.Errorf("couldn't marshal json: %v", err)
	}
	if _, err := fileWriter.Write(js); err != nil {
		return err
	}

	err = fileWriter.Flush()
	if err != nil {
		return fmt.Errorf("couldn't write data: %v", err)
	}

	err = sstFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content to disk: %v", err)
	}

	dirFile, err := os.Open(filepath.Dir(filename))
	if err != nil {
		return err
	}
	defer dirFile.Close()

	err = dirFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content(directory) to disk: %v", err)
	}

	return nil
}

func closeCompactionSSTable(sstFile *os.File, filename string) error {
	fileWriter := bufio.NewWriter(sstFile)
	if err := fileWriter.WriteByte(']'); err != nil {
		sstFile.Close()
		return err
	}
	if err := fileWriter.Flush(); err != nil {
		sstFile.Close()
		return err
	}
	if err := sstFile.Sync(); err != nil {
		sstFile.Close()
		return err
	}
	if err := sstFile.Close(); err != nil {
		return err
	}

	dirFile, err := os.Open(filepath.Dir(filename))
	if err != nil {
		return err
	}
	defer dirFile.Close()

	return dirFile.Sync()
}

func (e *Engine) deleteOldFiles(filenames []string) error {
	for _, filename := range filenames {
		err := os.Remove(filename)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseSSTRecords(filename string) ([]KV, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var records []KV
	err = json.Unmarshal(data, &records)
	if err != nil {
		return nil, err
	}

	return records, nil
}
