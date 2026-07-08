package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
)

type Engine struct {
	Store        map[string]string
	SSTfileCount int
	Manifest     string
	mu           sync.RWMutex
}

type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NewEngine() *Engine {
	return &Engine{
		Store:        make(map[string]string),
		SSTfileCount: 1,
		Manifest:     "MANIFEST",
	}
}

func (e *Engine) UpsertKeyValue(key, value string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Store[key] = value

	if len(e.Store) >= 2000 {
		err := e.writeSSTable()
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Engine) RetrieveKeyValue(key string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	value, ok := e.Store[key]
	if ok {
		return value, nil
	}

	manifestFile, err := os.OpenFile(e.Manifest, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("couldn't open manifest file: %v", err)
	}
	defer manifestFile.Close()

	manifastData, err := io.ReadAll(manifestFile)
	if err != nil {
		return "", fmt.Errorf("reading data: %v", err)
	}

	fileNames := strings.Fields(string(manifastData))
	for i := len(fileNames) - 1; i >= 0; i-- {
		fileName := fileNames[i]

		file, err := os.OpenFile(fileName, os.O_RDONLY, 0o644)
		if err != nil {
			log.Printf("couldn't open sst file: %v", err)
			return "", fmt.Errorf("couldn't open sst file: %v", err)
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			return "", fmt.Errorf("reading data: %v", err)
		}

		var KeyValues []KV

		err = json.Unmarshal(data, &KeyValues)
		if err != nil {
			return "", err
		}

		for _, kv := range KeyValues {
			if kv.Key == key {
				return kv.Value, nil
			}
		}
	}

	return "", fmt.Errorf("key does not exist")
}

func (e *Engine) writeSSTable() error {
	fileName := fmt.Sprintf("sst-%d.json", e.SSTfileCount)
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("couldn't open sst file: %v", err)
	}
	defer file.Close()

	manifestFile, err := os.OpenFile(e.Manifest, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("couldn't open manifest file: %v", err)
	}
	defer manifestFile.Close()

	manifestFile.Write([]byte(fileName + "\n"))
	e.SSTfileCount++

	keys := e.sortHashMap()

	var keyValues []KV
	for _, k := range keys {
		keyValues = append(keyValues, KV{
			Key:   k,
			Value: e.Store[k],
		})
	}

	js, err := json.Marshal(keyValues)
	if err != nil {
		return fmt.Errorf("couldn't marshal json: %v", err)
	}

	fileWriter := bufio.NewWriter(file)
	fileWriter.Write(js)

	err = fileWriter.Flush()
	if err != nil {
		return fmt.Errorf("couldn't write data: %v", err)
	}

	e.Store = make(map[string]string)
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
