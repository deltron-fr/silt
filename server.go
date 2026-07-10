package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/deltron-fr/silt/internal/engine"
)

type Server struct {
	engine *engine.Engine
}

func NewServer() *Server {
	kvEngine := engine.NewEngine()
	err := kvEngine.StartUp()
	if err != nil {
		log.Fatalf("an error occured at startup: %v", err)
	}

	return &Server{
		engine: kvEngine,
	}
}

func (s *Server) handleRetrieveKV(w http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")
	value, err := s.engine.RetrieveKeyValue(key)
	if err != nil {
		http.Error(w, fmt.Sprintf("key %s does not exist: %v", key, err), http.StatusNotFound)
		return
	}

	w.Write([]byte(value))
}

func (s *Server) handleCreateKV(w http.ResponseWriter, req *http.Request) {
	key := req.PathValue("key")

	data, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "couldn't read request body: %w", http.StatusInternalServerError)
		return
	}
	defer req.Body.Close()

	err = s.engine.UpsertKeyValue(key, string(data))
	if err != nil {
		http.Error(w, fmt.Sprintf("couldn't upsert key-value: %v", err), http.StatusInternalServerError)
		return
	}
}
