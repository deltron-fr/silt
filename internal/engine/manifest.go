package engine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (e *Engine) manifestStartup() error {
	manifestFile, err := os.Open(e.Manifest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, err = os.Create(e.Manifest)
			if err != nil {
				return err
			}

			return nil
		} else {
			return err
		}
	}
	defer manifestFile.Close()

	data, err := io.ReadAll(manifestFile)
	if err != nil {
		return fmt.Errorf("reading data from manifest: %v", err)
	}

	manifestData := strings.TrimSpace(string(data))
	if manifestData == "" {
		return nil
	}

	sstFileNames := strings.Fields(manifestData)

	lastFileName := sstFileNames[len(sstFileNames)-1]
	const (
		prefix = "sst-"
		suffix = ".json"
	)
	if !strings.HasPrefix(lastFileName, prefix) || !strings.HasSuffix(lastFileName, suffix) {
		return fmt.Errorf("invalid SSTable filename in manifest: %q", lastFileName)
	}

	fileCount, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(lastFileName, prefix), suffix))
	if err != nil {
		return err
	}

	e.SSTfileCount = fileCount + 1
	return nil
}

func (e *Engine) updateManifest(data []byte) error {
	// Replacing a fully synced temporary file prevents readers from seeing a
	// partially rewritten MANIFEST after a crash.
	tmpFile, err := os.OpenFile(e.Manifest+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	fileWriter := bufio.NewWriter(tmpFile)
	fileWriter.Write(data)

	err = fileWriter.Flush()
	if err != nil {
		return fmt.Errorf("couldn't write data: %v", err)
	}

	err = tmpFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content to disk: %v", err)
	}

	err = os.Rename(e.Manifest+".tmp", e.Manifest)
	if err != nil {
		return err
	}
	defer os.Remove(e.Manifest + ".tmp")

	dirFile, err := os.Open(filepath.Dir(e.Manifest))
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
