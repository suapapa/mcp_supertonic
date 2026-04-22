package audioserve

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// URI template registered with MCP (RFC 6570).
const TemplateString = "tts://output/{id}"

const uriPrefix = "tts://output/"

// Registry maps short-lived resource IDs to WAV files on disk for resources/read.
type Registry struct {
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]entry
}

type entry struct {
	path    string
	expires time.Time
}

// NewRegistry starts a background sweeper that drops expired IDs (files are not deleted).
func NewRegistry(ttl time.Duration) *Registry {
	r := &Registry{
		ttl: ttl,
		m:   make(map[string]entry),
	}
	go r.sweepLoop()
	return r
}

func (r *Registry) sweepLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.cleanup(time.Now())
	}
}

func (r *Registry) cleanup(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, e := range r.m {
		if now.After(e.expires) {
			delete(r.m, id)
		}
	}
}

// Register associates an absolute file path with a fresh tts://output/{uuid} URI.
func (r *Registry) Register(absPath string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := uuid.New().String()
	r.m[id] = entry{
		path:    absPath,
		expires: time.Now().Add(r.ttl),
	}
	return uriPrefix + id
}

// ReadBlob returns base64-encoded file bytes for MCP BlobResourceContents, or an error if expired/unknown.
func (r *Registry) ReadBlob(uri string) (blobB64 string, mime string, err error) {
	id := strings.TrimPrefix(uri, uriPrefix)
	if id == "" || strings.Contains(id, "/") {
		return "", "", fmt.Errorf("invalid tts resource uri")
	}
	r.mu.RLock()
	e, ok := r.m[id]
	r.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return "", "", fmt.Errorf("resource expired or unknown: %s", uri)
	}
	raw, err := os.ReadFile(e.path)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(raw), "audio/wav", nil
}
