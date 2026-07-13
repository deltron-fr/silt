package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

const NotExist = "Null"

type Engine struct {
	Store         map[string]string
	SSTfileCount  int
	Manifest      string
	NegativeCache *LRUCache
	WAL           string
	mu            sync.RWMutex
}

type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NewEngine() *Engine {
	return &Engine{
		NegativeCache: NewCache(100),
		Store:         make(map[string]string),
		Manifest:      "MANIFEST",
		WAL:           "wal.db",
	}
}

func (e *Engine) StartUp() error {
	var err error

	err = e.manifestStartup()
	if err != nil {
		return err
	}

	err = e.replayWAL()
	if err != nil {
		return err
	}

	return nil
}

func (e *Engine) UpsertKeyValue(key, value string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Store[key] = value

	if e.NegativeCache.Exists(key) {
		e.NegativeCache.Delete(key)
	}

	err := e.runWAL(key, value)
	if err != nil {
		return err
	}

	if len(e.Store) >= 2000 {
		err = e.writeSSTable()
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Engine) RetrieveKeyValue(key string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	v := e.NegativeCache.Get(key)
	if v == NotExist {
		return "", fmt.Errorf("key does not exist")
	}

	value, ok := e.Store[key]
	if ok {
		return value, nil
	}

	retrievedValue, err := e.retrieveFromSST(key)
	if err != nil {
		return "", err
	}

	if retrievedValue == "" {
		e.NegativeCache.Put(key, NotExist)
		return "", fmt.Errorf("key does not exist")
	}

	return retrievedValue, nil
}

func (e *Engine) retrieveFromSST(key string) (string, error) {
	var value string

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

		sstFile, err := os.OpenFile(fileName, os.O_RDONLY, 0o644)
		if err != nil {
			log.Printf("couldn't open sst file: %v", err)
			return "", fmt.Errorf("couldn't open sst file: %v", err)
		}
		defer sstFile.Close()

		data, err := io.ReadAll(sstFile)
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
				value = kv.Value
				break
			}
		}

		if value != "" {
			break
		}
	}

	return value, nil
}
