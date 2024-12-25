package gohlslib

import (
	"net/http"
	"path/filepath"
	"sync"
)

type muxerServer struct {
	mutex        sync.RWMutex
	pathHandlers map[string]http.HandlerFunc
}

func (s *muxerServer) initialize() {
	s.pathHandlers = make(map[string]http.HandlerFunc)
}

func (s *muxerServer) registerPath(path string, cb http.HandlerFunc) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.pathHandlers[path] = cb
}

func (s *muxerServer) unregisterPath(path string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.pathHandlers, path)
}

func (s *muxerServer) getPathHandler(path string) http.HandlerFunc {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.pathHandlers[path]
}

func (s *muxerServer) handle(w http.ResponseWriter, r *http.Request) {
	path := filepath.Base(r.URL.Path)

	s.mutex.RLock()
	handler, ok := s.pathHandlers[path]
	s.mutex.RUnlock()

	if ok {
		handler(w, r)
	}
}
