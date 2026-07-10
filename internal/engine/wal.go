package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

type WALRecord struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (e *Engine) replayWAL() error {
	file, err := os.Open(e.WAL)
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
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("reading data: %v", err)
	}

	jsParts := bytes.Split(data, []byte("\n"))
	if len(jsParts) == 0 {
		return fmt.Errorf("invalid wal file format")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for i, js := range jsParts {

		// TODO: revisit this condition
		if i == len(jsParts)-1 {
			break
		}

		var record WALRecord
		err = json.Unmarshal(js, &record)
		if err != nil {
			return err
		}

		e.Store[record.Key] = record.Value
	}

	return nil
}

func (e *Engine) runWAL(key, value string) error {
	WALFile, err := os.OpenFile(e.WAL, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("couldn't open manifest file: %v", err)
	}
	defer WALFile.Close()

	record := WALRecord{
		Op:    "put",
		Key:   key,
		Value: value,
	}

	js, err := json.Marshal(&record)
	if err != nil {
		return err
	}

	js = append(js, byte('\n'))

	fileWriter := bufio.NewWriter(WALFile)
	fileWriter.Write(js)

	err = fileWriter.Flush()
	if err != nil {
		return fmt.Errorf("couldn't write data: %v", err)
	}

	err = WALFile.Sync()
	if err != nil {
		return fmt.Errorf("couldn't flush file content to disk: %v", err)
	}

	return nil
}
