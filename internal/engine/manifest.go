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
			_, err = os.Create(e.WAL)
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

	sstFileNames := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(sstFileNames) == 0 {
		return nil
	}

	fmt.Println(sstFileNames)

	lastFileName := sstFileNames[len(sstFileNames)-1]
	fmt.Println("last file name: ", lastFileName)

	fileCount, err := strconv.Atoi(lastFileName[4 : len(lastFileName)-5])
	if err != nil {
		return err
	}

	e.SSTfileCount = fileCount + 1
	return nil
}

func (e *Engine) updateManifest(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek to start of manifest: %v", err)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("reading data: %v", err)
	}

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
