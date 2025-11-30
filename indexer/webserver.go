package indexer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sourcegraph/zoekt/search"
	"github.com/sourcegraph/zoekt/web"
)

// WebServerManager manages the Zoekt web server lifecycle
type WebServerManager struct {
	mu        sync.Mutex
	server    *http.Server
	indexDir  string
	port      int
	running   bool
	startedAt time.Time
}

// WebServerStatus contains information about the web server state
type WebServerStatus struct {
	Running   bool      `json:"running"`
	Port      int       `json:"port,omitempty"`
	URL       string    `json:"url,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

// NewWebServerManager creates a new web server manager
func NewWebServerManager(indexDir string) *WebServerManager {
	return &WebServerManager{
		indexDir: indexDir,
	}
}

// Start starts the Zoekt web server on the specified port
// If port is 0, a random available port will be used
func (m *WebServerManager) Start(port int) (*WebServerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil, fmt.Errorf("web server is already running on port %d", m.port)
	}

	// Create a searcher for the index directory
	searcher, err := search.NewDirectorySearcher(m.indexDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create searcher: %w", err)
	}

	// Create the web server
	webServer := &web.Server{
		Searcher: searcher,
		HTML:     true,
		RPC:      true,
		Print:    true,
		Version:  "code-index-mcp",
		Top:      web.Top,
	}

	// Parse templates
	for name, text := range web.TemplateText {
		if _, err := webServer.Top.New(name).Parse(text); err != nil {
			searcher.Close()
			return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
		}
	}

	// Create the HTTP mux
	mux, err := web.NewMux(webServer)
	if err != nil {
		searcher.Close()
		return nil, fmt.Errorf("failed to create mux: %w", err)
	}

	// Find an available port if port is 0
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		searcher.Close()
		return nil, fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	// Get the actual port (useful when port was 0)
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Create HTTP server
	m.server = &http.Server{
		Handler: mux,
	}

	m.port = actualPort
	m.running = true
	m.startedAt = time.Now()

	// Start serving in a goroutine
	go func() {
		if err := m.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Server stopped unexpectedly
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}
	}()

	return &WebServerStatus{
		Running:   true,
		Port:      actualPort,
		URL:       fmt.Sprintf("http://127.0.0.1:%d", actualPort),
		StartedAt: m.startedAt,
	}, nil
}

// Stop stops the Zoekt web server
func (m *WebServerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("web server is not running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	m.running = false
	m.server = nil
	m.port = 0

	return nil
}

// Status returns the current status of the web server
func (m *WebServerManager) Status() *WebServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return &WebServerStatus{Running: false}
	}

	return &WebServerStatus{
		Running:   true,
		Port:      m.port,
		URL:       fmt.Sprintf("http://127.0.0.1:%d", m.port),
		StartedAt: m.startedAt,
	}
}
