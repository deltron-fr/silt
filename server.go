package main

import (
	"io"
	"net/http"
)

type Server struct {
	engine *Engine
}

func NewServer() *Server {
	return &Server{
		engine: NewEngine(),
	}
}

func (s *Server) handleRetrieveKV(w http.ResponseWriter, req *http.Request) {
	value, err := s.engine.RetrieveKeyValue(req.PathValue("key"))
	if err != nil {
		http.Error(w, "key does not exist", http.StatusNotFound)
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
		http.Error(w, "couldn't upsert key-value", http.StatusInternalServerError)
		return
	}
}
