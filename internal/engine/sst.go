package engine

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

func (e *Engine) writeSSTable() error {
	if e.SSTfileCount == 0 {
		e.SSTfileCount++
	}

	fileName := fmt.Sprintf("sst-%d.json", e.SSTfileCount)
	sstFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("couldn't open sst file: %v", err)
	}
	defer sstFile.Close()

	manifestFile, err := os.Open(e.Manifest)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("couldn't open manifest file: %v", err)
		}
	}
	defer manifestFile.Close()

	currentManifestData, err := io.ReadAll(manifestFile)
	if err != nil {
		return err
	}

	manifestData := append(currentManifestData, []byte(fileName+"\n")...)

	keys := e.sortHashMap()

	var keyValues []KV
	for _, k := range keys {
		value := e.Store[k]
		if value.op == OpDelete {
			keyValues = append(keyValues, KV{
				Key:     k,
				Deleted: true,
			})
		} else {
			keyValues = append(keyValues, KV{
				Key:   k,
				Value: e.Store[k].value,
			})
		}
	}

	js, err := json.Marshal(keyValues)
	if err != nil {
		return fmt.Errorf("couldn't marshal json: %v", err)
	}

	fileWriter := bufio.NewWriter(sstFile)
	fileWriter.Write(js)

	err = fileWriter.Flush()
	if err != nil {
		return fmt.Errorf("couldn't write data: %v", err)
	}

	err = sstFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content to disk: %v", err)
	}

	dirFile, err := os.Open(filepath.Dir(fileName))
	if err != nil {
		return err
	}
	defer dirFile.Close()

	err = dirFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content(directory) to disk: %v", err)
	}

	// Publish the SSTable in the MANIFEST before clearing the WAL. If the
	// process stops between these steps, replaying the WAL is still harmless.
	err = e.updateManifest(manifestData)
	if err != nil {
		return fmt.Errorf("couldn't update manifest file: %v", err)
	}
	e.SSTfileCount++

	e.Store = make(map[string]Value)
	WALFile, err := os.OpenFile(e.WAL, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer WALFile.Close()

	err = WALFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content to disk: %v", err)
	}

	return nil
}

func (e *Engine) sortHashMap() []string {
	keys := make([]string, 0, len(e.Store))

	for k := range e.Store {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}
