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

const (
	NotExist = "Null"
	OpDelete = "delete"
	OpPut    = "put"
)

type Engine struct {
	Store         map[string]Value
	Manifest      string
	WAL           string
	SSTfileCount  int
	OpCounter     int
	NegativeCache *LRUCache
	mu            sync.RWMutex
}

type KV struct {
	Key     string `json:"key"`
	Value   string `json:"value,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

type Value struct {
	value string
	op    string
}

func NewEngine() *Engine {
	return &Engine{
		NegativeCache: NewCache(100),
		Store:         make(map[string]Value),
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

	e.OpCounter++

	if e.NegativeCache.Exists(key) {
		e.NegativeCache.Delete(key)
	}

	err := e.runWAL(key, value, OpPut)
	if err != nil {
		return err
	}

	e.Store[key] = Value{value: value, op: OpPut}

	if len(e.Store) >= 2000 {
		err = e.writeSSTable()
		if err != nil {
			return err
		}
	}

	if e.OpCounter >= 10000 {
		err = e.Compaction()
		if err != nil {
			return err
		}
		e.OpCounter = 0
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

	if value.op == OpDelete {
		return "", fmt.Errorf("key does not exist")
	}

	if ok {
		return value.value, nil
	}

	retrievedValue, found, err := e.retrieveFromSST(key)
	if err != nil {
		return "", err
	}

	if !found {
		e.NegativeCache.Put(key, NotExist)
		return "", fmt.Errorf("key does not exist")
	}

	return retrievedValue, nil
}

func (e *Engine) DeleteKeyValue(key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	err := e.runWAL(key, "", "delete")
	if err != nil {
		return err
	}

	e.Store[key] = Value{op: OpDelete}
	e.OpCounter++

	if e.OpCounter >= 10000 {
		err = e.Compaction()
		if err != nil {
			return err
		}
		e.OpCounter = 0
	}

	return nil
}

func (e *Engine) retrieveFromSST(key string) (string, bool, error) {
	var value string
	found := false
	deleted := false

	manifestFile, err := os.OpenFile(e.Manifest, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return "", false, fmt.Errorf("couldn't open manifest file: %v", err)
	}
	defer manifestFile.Close()

	manifastData, err := io.ReadAll(manifestFile)
	if err != nil {
		return "", false, fmt.Errorf("reading data: %v", err)
	}

	fileNames := strings.Fields(string(manifastData))
	for i := len(fileNames) - 1; i >= 0; i-- {
		fileName := fileNames[i]

		sstFile, err := os.OpenFile(fileName, os.O_RDONLY, 0o644)
		if err != nil {
			log.Printf("couldn't open sst file: %v", err)
			return "", false, fmt.Errorf("couldn't open sst file: %v", err)
		}
		defer sstFile.Close()

		data, err := io.ReadAll(sstFile)
		if err != nil {
			return "", false, fmt.Errorf("reading data: %v", err)
		}

		var KeyValues []KV

		err = json.Unmarshal(data, &KeyValues)
		if err != nil {
			return "", false, err
		}

		for _, kv := range KeyValues {
			if kv.Key == key {
				if kv.Deleted {
					deleted = true
					break
				} else {
					found = true
					value = kv.Value
				}
				break
			}
		}

		if found || deleted {
			break
		}
	}

	return value, found, nil
}
