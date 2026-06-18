package chroma

import (
	"fmt"
	"strings"
)

// ServerConfig holds configuration for a Chroma server.
type ServerConfig struct {
	// Network
	Port                int
	ListenAddress       string
	MaxPayloadSizeBytes int
	CORSAllowOrigins    []string

	// Storage
	PersistPath    string
	SQLiteFilename string
	AllowReset     bool

	// OpenTelemetry (optional)
	OTelEndpoint    string
	OTelServiceName string

	// Raw config (takes precedence if set)
	rawYAML string
}

// ServerOption is a function that configures a ServerConfig.
type ServerOption func(*ServerConfig)

// DefaultServerConfig returns a ServerConfig with default values matching Chroma's defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:                8000,
		ListenAddress:       "127.0.0.1",
		MaxPayloadSizeBytes: 40 * 1024 * 1024, // 40 MB
		PersistPath:         "./chroma",
		SQLiteFilename:      "chroma.sqlite3",
		AllowReset:          false,
	}
}

// WithPort sets the server port.
func WithPort(port int) ServerOption {
	return func(c *ServerConfig) {
		c.Port = port
	}
}

// WithListenAddress sets the address to bind to.
func WithListenAddress(addr string) ServerOption {
	return func(c *ServerConfig) {
		c.ListenAddress = addr
	}
}

// WithMaxPayloadSize sets the maximum request payload size in bytes.
func WithMaxPayloadSize(bytes int) ServerOption {
	return func(c *ServerConfig) {
		c.MaxPayloadSizeBytes = bytes
	}
}

// WithCORSAllowOrigins sets allowed CORS origins.
func WithCORSAllowOrigins(origins ...string) ServerOption {
	return func(c *ServerConfig) {
		c.CORSAllowOrigins = origins
	}
}

// WithPersistPath sets the data persistence directory.
func WithPersistPath(path string) ServerOption {
	return func(c *ServerConfig) {
		c.PersistPath = path
	}
}

// WithSQLiteFilename sets the SQLite database filename.
func WithSQLiteFilename(filename string) ServerOption {
	return func(c *ServerConfig) {
		c.SQLiteFilename = filename
	}
}

// WithAllowReset enables the reset endpoint (use with caution).
func WithAllowReset(allow bool) ServerOption {
	return func(c *ServerConfig) {
		c.AllowReset = allow
	}
}

// WithOpenTelemetry configures OpenTelemetry tracing.
func WithOpenTelemetry(endpoint, serviceName string) ServerOption {
	return func(c *ServerConfig) {
		c.OTelEndpoint = endpoint
		c.OTelServiceName = serviceName
	}
}

// WithRawYAML sets a raw YAML config string (overrides all other options).
func WithRawYAML(yaml string) ServerOption {
	return func(c *ServerConfig) {
		c.rawYAML = yaml
	}
}

// toYAML converts the config to a YAML string.
func (c *ServerConfig) toYAML() string {
	if c.rawYAML != "" {
		return c.rawYAML
	}

	var b strings.Builder

	fmt.Fprintf(&b, "port: %d\n", c.Port)
	fmt.Fprintf(&b, "listen_address: %q\n", c.ListenAddress)
	fmt.Fprintf(&b, "max_payload_size_bytes: %d\n", c.MaxPayloadSizeBytes)
	fmt.Fprintf(&b, "persist_path: %q\n", c.PersistPath)
	fmt.Fprintf(&b, "sqlite_filename: %q\n", c.SQLiteFilename)
	fmt.Fprintf(&b, "allow_reset: %t\n", c.AllowReset)

	if len(c.CORSAllowOrigins) > 0 {
		b.WriteString("cors_allow_origins:\n")
		for _, origin := range c.CORSAllowOrigins {
			fmt.Fprintf(&b, "  - %q\n", origin)
		}
	}

	if c.OTelEndpoint != "" {
		b.WriteString("open_telemetry:\n")
		fmt.Fprintf(&b, "  endpoint: %q\n", c.OTelEndpoint)
		if c.OTelServiceName != "" {
			fmt.Fprintf(&b, "  service_name: %q\n", c.OTelServiceName)
		}
	}

	return b.String()
}

// NewServer creates and starts a new Chroma server with the given options.
func NewServer(opts ...ServerOption) (*Server, error) {
	cfg := DefaultServerConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return StartServer(StartServerConfig{
		ConfigString: cfg.toYAML(),
	})
}
