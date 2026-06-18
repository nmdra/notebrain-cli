package tokenizers

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

const (
	HFDefaultRevision   = "main"
	HFDefaultTimeout    = 30 * time.Second
	HFMaxRetries        = 3
	HFRetryDelay        = time.Second
	hfDefaultHubBaseURL = "https://huggingface.co"
	// Deprecated: HFHubBaseURL is immutable for backward compatibility.
	// Use WithHFBaseURL to override the endpoint per tokenizer instance.
	HFHubBaseURL = hfDefaultHubBaseURL
	// HFMaxRetryAfterDelay caps the maximum delay from Retry-After headers
	// to prevent excessive waits from misconfigured or malicious servers
	HFMaxRetryAfterDelay = 5 * time.Minute

	// DefaultMaxTokenizerSize is the default maximum size for tokenizer files (500MB)
	// This prevents OOM errors from excessively large downloads
	DefaultMaxTokenizerSize = 500 * 1024 * 1024

	// HTTP connection pooling defaults
	defaultMaxIdleConns        = 100
	defaultMaxIdleConnsPerHost = 10
	defaultIdleTimeout         = 90 * time.Second

	// HTTP connection pooling maximum bounds
	// These limits prevent resource exhaustion from misconfiguration
	maxAllowedIdleConns        = 1000 // Maximum total idle connections across all hosts
	maxAllowedIdleConnsPerHost = 100  // Maximum idle connections per individual host
)

var (
	libraryVersion = "0.1.0" // Default version, will be set from library if available

	// Shared HTTP client for HuggingFace downloads with connection pooling
	hfHTTPClient *http.Client
	hfClientOnce sync.Once
	jitterNonce  atomic.Int64

	// ErrCacheNotFound is returned when a requested cache file does not exist
	ErrCacheNotFound = errors.New("cache file not found")
)

// GetLibraryVersion returns the current library version used in User-Agent
func GetLibraryVersion() string {
	return libraryVersion
}

// SetLibraryVersion sets the library version for User-Agent headers
func SetLibraryVersion(version string) {
	if version != "" {
		libraryVersion = version
	}
}

// getEnvIntValue is a generic helper for parsing integer environment variables
func getEnvIntValue[T int | int64](key string, defaultValue T, parser func(string) (T, error)) T {
	envVal := os.Getenv(key)
	if envVal == "" {
		return defaultValue
	}

	val, err := parser(envVal)
	if err != nil {
		// #nosec G706 -- key/envVal are sanitized before logging.
		log.Printf("[WARNING] Invalid integer value for %s: '%s' (error: %v), using default: %v\n",
			sanitizeLogValue(key), sanitizeLogValue(envVal), err, defaultValue)
		return defaultValue
	}

	if val <= 0 {
		log.Printf("[WARNING] Non-positive value for %s: %v, using default: %v\n",
			key, val, defaultValue)
		return defaultValue
	}

	return val
}

func sanitizeLogValue(v string) string {
	v = strings.ReplaceAll(v, "\n", "\\n")
	v = strings.ReplaceAll(v, "\r", "\\r")
	return v
}

// getEnvInt retrieves an integer value from environment variable
func getEnvInt(key string, defaultValue int) int {
	return getEnvIntValue(key, defaultValue, func(s string) (int, error) {
		return strconv.Atoi(s)
	})
}

// getEnvInt64 retrieves an int64 value from environment variable
func getEnvInt64(key string, defaultValue int64) int64 {
	return getEnvIntValue(key, defaultValue, func(s string) (int64, error) {
		return strconv.ParseInt(s, 10, 64)
	})
}

// getEnvDuration retrieves a duration value from environment variable
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if envVal := os.Getenv(key); envVal != "" {
		if val, err := time.ParseDuration(envVal); err == nil && val > 0 {
			return val
		} else if err != nil {
			// Always log warning for invalid configuration to help users debug
			// #nosec G706 -- key/envVal are sanitized before logging.
			log.Printf("[WARNING] Invalid duration value for %s: '%s' (error: %v), using default: %v\n",
				sanitizeLogValue(key), sanitizeLogValue(envVal), err, defaultValue)
		} else if val <= 0 {
			// Log warning for non-positive durations
			// #nosec G706 -- key is sanitized before logging.
			log.Printf("[WARNING] Non-positive duration for %s: %v, using default: %v\n",
				sanitizeLogValue(key), val, defaultValue)
		}
	}
	return defaultValue
}

// validateHTTPPoolingConfig validates and adjusts HTTP pooling configuration for logical consistency
func validateHTTPPoolingConfig(maxIdleConns, maxIdleConnsPerHost int) (int, int) {
	originalMaxIdleConns := maxIdleConns
	originalMaxIdleConnsPerHost := maxIdleConnsPerHost

	// Ensure maxIdleConns is at least as large as maxIdleConnsPerHost
	// This is logical since total idle connections should be >= per-host idle connections
	if maxIdleConns < maxIdleConnsPerHost {
		maxIdleConns = maxIdleConnsPerHost
		// Always log this important logical adjustment
		log.Printf("[WARNING] HTTPMaxIdleConns (%d) was less than HTTPMaxIdleConnsPerHost (%d), adjusted to %d for consistency",
			originalMaxIdleConns, maxIdleConnsPerHost, maxIdleConns)
	}

	// Ensure reasonable bounds to prevent resource exhaustion
	if maxIdleConns > maxAllowedIdleConns {
		maxIdleConns = maxAllowedIdleConns
		log.Printf("[WARNING] HTTPMaxIdleConns (%d) exceeds maximum allowed (%d), capped to prevent resource exhaustion",
			originalMaxIdleConns, maxAllowedIdleConns)
	}
	if maxIdleConnsPerHost > maxAllowedIdleConnsPerHost {
		maxIdleConnsPerHost = maxAllowedIdleConnsPerHost
		log.Printf("[WARNING] HTTPMaxIdleConnsPerHost (%d) exceeds maximum allowed (%d), capped to prevent resource exhaustion",
			originalMaxIdleConnsPerHost, maxAllowedIdleConnsPerHost)
	}

	return maxIdleConns, maxIdleConnsPerHost
}

// initHFHTTPClient initializes the shared HTTP client with connection pooling.
// NOTE: Due to thread-safety via sync.Once, configuration changes after the first
// client initialization will not take effect. The HTTP client is initialized once
// per process lifetime.
func initHFHTTPClient(config *HFConfig) {
	hfClientOnce.Do(func() {
		// Apply configuration with priority: config fields > env vars > defaults
		maxIdleConns := config.HTTPMaxIdleConns
		if maxIdleConns == 0 {
			maxIdleConns = getEnvInt("HF_HTTP_MAX_IDLE_CONNS", defaultMaxIdleConns)
		}

		maxIdleConnsPerHost := config.HTTPMaxIdleConnsPerHost
		if maxIdleConnsPerHost == 0 {
			maxIdleConnsPerHost = getEnvInt("HF_HTTP_MAX_IDLE_CONNS_PER_HOST", defaultMaxIdleConnsPerHost)
		}

		// Store original values for logging
		originalMaxIdleConns := maxIdleConns
		originalMaxIdleConnsPerHost := maxIdleConnsPerHost

		// Validate and adjust configuration for logical consistency
		maxIdleConns, maxIdleConnsPerHost = validateHTTPPoolingConfig(maxIdleConns, maxIdleConnsPerHost)

		idleTimeout := config.HTTPIdleTimeout
		if idleTimeout == 0 {
			idleTimeout = getEnvDuration("HF_HTTP_IDLE_TIMEOUT", defaultIdleTimeout)
		}

		// Log final configuration in debug mode
		if os.Getenv("DEBUG") != "" {
			log.Printf("[DEBUG] HTTP Client Configuration:\n")
			log.Printf("  MaxIdleConns: %d", maxIdleConns)
			if originalMaxIdleConns != maxIdleConns {
				log.Printf("    (adjusted from %d for consistency)", originalMaxIdleConns)
			}
			log.Printf("  MaxIdleConnsPerHost: %d", maxIdleConnsPerHost)
			if originalMaxIdleConnsPerHost != maxIdleConnsPerHost {
				log.Printf("    (adjusted from %d due to bounds)", originalMaxIdleConnsPerHost)
			}
			log.Printf("  IdleTimeout: %v", idleTimeout)
		}

		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			// IdleConnTimeout is suitable for long-running processes that may
			// have periods of inactivity between downloads. For short scripts that
			// exit quickly, connections will be closed automatically on program exit.
			IdleConnTimeout:       idleTimeout,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

		hfHTTPClient = &http.Client{
			Transport: transport,
			// Default timeout - will be overridden per-request using context
			Timeout: 0,
		}
	})
}

// getHFHTTPClient returns the shared HTTP client for HuggingFace downloads
func getHFHTTPClient(config *HFConfig) *http.Client {
	initHFHTTPClient(config)
	return hfHTTPClient
}

// HFConfig holds HuggingFace-specific configuration
type HFConfig struct {
	Token       string
	Revision    string
	CacheDir    string
	Timeout     time.Duration
	MaxRetries  int
	OfflineMode bool
	// UseLocalCache enables checking the HuggingFace hub cache before downloading
	UseLocalCache bool
	// CacheTTL specifies how long cached tokenizers are considered valid (0 = forever)
	CacheTTL time.Duration
	// MaxTokenizerSize is the maximum allowed size for tokenizer files in bytes
	// (env: HF_MAX_TOKENIZER_SIZE, default: 500MB).
	// When set to 0 (zero value), falls back to HF_MAX_TOKENIZER_SIZE environment variable,
	// or DefaultMaxTokenizerSize (500MB) if the environment variable is not set.
	// Use WithHFMaxTokenizerSize to explicitly set this value.
	MaxTokenizerSize int64
	baseURL          string

	// HTTP client pooling configuration
	// These settings control connection reuse for improved performance.
	// Config fields take priority over environment variables.
	//
	// IMPORTANT: The HTTP client is initialized once per process using sync.Once.
	// Changes to these configuration values after the first HuggingFace download
	// will NOT take effect. Set these values before any HuggingFace operations.
	//
	// Performance trade-offs:
	// - Higher values: Better connection reuse, reduced latency for subsequent requests, but increased memory usage
	// - Lower values: Reduced memory footprint, but more connection establishment overhead
	//
	// Recommended configurations:
	// - High-throughput services: Increase HTTPMaxIdleConnsPerHost (e.g., 20-50) for parallel downloads
	// - Resource-constrained environments: Reduce both values (e.g., 50/5) to minimize memory usage
	// - Short-lived scripts: Reduce HTTPIdleTimeout (e.g., 10s) to release resources quickly
	//
	// Note: HTTPMaxIdleConns will be automatically adjusted to be >= HTTPMaxIdleConnsPerHost for logical consistency
	//
	// Debug mode: Set DEBUG=1 environment variable to see actual configuration values being used
	HTTPMaxIdleConns        int           // Maximum idle connections across all hosts (env: HF_HTTP_MAX_IDLE_CONNS, default: 100, max: 1000)
	HTTPMaxIdleConnsPerHost int           // Maximum idle connections per host (env: HF_HTTP_MAX_IDLE_CONNS_PER_HOST, default: 10, max: 100)
	HTTPIdleTimeout         time.Duration // How long to keep idle connections open (env: HF_HTTP_IDLE_TIMEOUT, default: 90s)
}

// SetBaseURL validates and sets a custom HuggingFace base URL.
// Leading/trailing whitespace and trailing slashes are removed.
func (c *HFConfig) SetBaseURL(baseURL string) error {
	normalized, err := normalizeHFBaseURL(baseURL)
	if err != nil {
		return err
	}
	c.baseURL = normalized
	return nil
}

// GetBaseURL returns the configured custom HuggingFace base URL.
// Returns an empty string when no custom URL is configured.
func (c *HFConfig) GetBaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

func normalizeHFBaseURL(baseURL string) (string, error) {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" {
		return "", errors.New("base URL cannot be empty")
	}
	parsedURL, err := url.Parse(normalized)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse base URL")
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", errors.New("base URL must be a valid absolute URL")
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("base URL scheme must be http or https")
	}
	return strings.TrimRight(normalized, "/"), nil
}

func resolveHFBaseURL(config *HFConfig) (string, error) {
	if config == nil || strings.TrimSpace(config.baseURL) == "" {
		return hfDefaultHubBaseURL, nil
	}
	normalized, err := normalizeHFBaseURL(config.baseURL)
	if err != nil {
		return "", errors.Wrap(err, "invalid HF base URL configuration")
	}
	return normalized, nil
}

// FromHuggingFace loads a tokenizer from HuggingFace Hub using the model identifier.
//
// The model identifier can be in the format "organization/model" or just "model".
// For example: "bert-base-uncased", "google/flan-t5-base", "meta-llama/Llama-2-7b-hf".
//
// By default, it loads from the "main" branch/revision. Use WithHFRevision to specify
// a different revision (branch, tag, or commit hash).
//
// For private or gated models, authentication is required. Set the HF_TOKEN environment
// variable or use WithHFToken option.
//
// The tokenizer is cached locally for faster subsequent loads. The cache location is
// platform-specific and can be overridden with WithHFCacheDir.
//
// Example:
//
//	tokenizer, err := FromHuggingFace("bert-base-uncased")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer tokenizer.Close()
func FromHuggingFace(modelID string, opts ...TokenizerOption) (*Tokenizer, error) {
	if modelID == "" {
		return nil, errors.New("model ID cannot be empty")
	}

	// Validate model ID format
	if err := validateModelID(modelID); err != nil {
		return nil, errors.Wrapf(err, "invalid model ID: %s", modelID)
	}

	// Create tokenizer with HF config
	tokenizer := &Tokenizer{
		defaultEncodingOpts: EncodeOptions{
			ReturnTokens: true,
		},
		hfConfig: &HFConfig{
			Revision:   HFDefaultRevision,
			Timeout:    HFDefaultTimeout,
			MaxRetries: HFMaxRetries,
		},
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(tokenizer); err != nil {
			return nil, errors.Wrapf(err, "failed to apply tokenizer option")
		}
	}

	if tokenizer.hfConfig.Revision == "" {
		tokenizer.hfConfig.Revision = HFDefaultRevision
	}
	if err := validateHFRevision(tokenizer.hfConfig.Revision); err != nil {
		return nil, errors.Wrap(err, "invalid HuggingFace revision")
	}

	// Get token from environment if not provided
	if tokenizer.hfConfig.Token == "" {
		tokenizer.hfConfig.Token = os.Getenv("HF_TOKEN")
	}

	// Enable HF cache checking by default unless explicitly disabled
	if !tokenizer.hfConfig.UseLocalCache && os.Getenv("HF_USE_LOCAL_CACHE") != "false" {
		tokenizer.hfConfig.UseLocalCache = true
	}

	// Try cache lookup hierarchy:
	// 1. Pure-tokenizers cache
	cachedPath := getHFCachePath(tokenizer.hfConfig.CacheDir, modelID, tokenizer.hfConfig.Revision)
	if data, err := loadFromCacheWithValidation(cachedPath, tokenizer.hfConfig.CacheTTL); err == nil {
		return FromBytes(data, opts...)
	}

	// 2. HuggingFace hub cache (if enabled)
	if tokenizer.hfConfig.UseLocalCache {
		if data, err := checkHFHubCache(modelID, tokenizer.hfConfig.Revision); err == nil {
			// Save to our cache for faster future access
			if cacheErr := saveToHFCache(cachedPath, data); cacheErr != nil {
				log.Printf("[WARNING] Failed to save HuggingFace tokenizer cache at %s: %v", cachedPath, cacheErr)
			}
			return FromBytes(data, opts...)
		}
	}

	// 3. Offline mode check
	if tokenizer.hfConfig.OfflineMode {
		return nil, errors.New("offline mode enabled but tokenizer not found in any cache")
	}

	// Download tokenizer.json from HuggingFace
	data, err := downloadTokenizerFromHF(modelID, tokenizer.hfConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download tokenizer from HuggingFace")
	}

	// Save to cache
	if err := saveToHFCache(cachedPath, data); err != nil {
		log.Printf("[WARNING] Failed to save HuggingFace tokenizer cache at %s: %v", cachedPath, err)
	}

	// Create tokenizer from downloaded data
	return FromBytes(data, opts...)
}

// WithHFToken sets the HuggingFace API token for authentication
func WithHFToken(token string) TokenizerOption {
	return func(t *Tokenizer) error {
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.Token = token
		return nil
	}
}

// WithHFRevision sets the model revision (branch, tag, or commit hash)
func WithHFRevision(revision string) TokenizerOption {
	return func(t *Tokenizer) error {
		normalized := strings.TrimSpace(revision)
		if err := validateHFRevision(normalized); err != nil {
			return err
		}
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.Revision = normalized
		return nil
	}
}

// WithHFCacheDir sets a custom cache directory for HuggingFace tokenizers
func WithHFCacheDir(dir string) TokenizerOption {
	return func(t *Tokenizer) error {
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.CacheDir = dir
		return nil
	}
}

// WithHFBaseURL sets a custom HuggingFace base URL.
// This is primarily useful for tests and custom mirrors.
// Leading/trailing whitespace and trailing slashes are removed.
func WithHFBaseURL(baseURL string) TokenizerOption {
	return func(t *Tokenizer) error {
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		return t.hfConfig.SetBaseURL(baseURL)
	}
}

// WithHFTimeout sets the download timeout for HuggingFace requests
func WithHFTimeout(timeout time.Duration) TokenizerOption {
	return func(t *Tokenizer) error {
		if timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.Timeout = timeout
		return nil
	}
}

// WithHFOfflineMode forces the tokenizer to only use cached versions
func WithHFOfflineMode(offline bool) TokenizerOption {
	return func(t *Tokenizer) error {
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.OfflineMode = offline
		return nil
	}
}

// WithHFMaxTokenizerSize sets the maximum allowed size for tokenizer files in bytes
// Default is 500MB. Set to a very large value to effectively disable size validation.
func WithHFMaxTokenizerSize(maxSize int64) TokenizerOption {
	return func(t *Tokenizer) error {
		if maxSize < 0 {
			return errors.New("max tokenizer size must be non-negative")
		}
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.MaxTokenizerSize = maxSize
		return nil
	}
}

// downloadTokenizerFromHF downloads the tokenizer.json file from HuggingFace Hub
func downloadTokenizerFromHF(modelID string, config *HFConfig) ([]byte, error) {
	baseURL, err := resolveHFBaseURL(config)
	if err != nil {
		return nil, err
	}
	revision := strings.TrimSpace(config.Revision)
	if revision == "" {
		revision = HFDefaultRevision
	}
	if err := validateHFRevision(revision); err != nil {
		return nil, errors.Wrap(err, "invalid HuggingFace revision")
	}
	url := fmt.Sprintf("%s/%s/resolve/%s/tokenizer.json", baseURL, modelID, revision)

	var lastErr error
	var retryAfterDuration time.Duration
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		if attempt > 0 {
			var delay time.Duration

			// Use server-suggested delay if available
			if retryAfterDuration > 0 {
				delay = retryAfterDuration
				if os.Getenv("DEBUG") != "" {
					log.Printf("[DEBUG] Retry attempt %d: using server-suggested delay from Retry-After header (%v)",
						attempt, delay)
				}
				// Reset for next iteration
				retryAfterDuration = 0
			} else {
				// Exponential backoff with jitter
				baseDelay := HFRetryDelay * (1 << (attempt - 1))
				// Add 0-25% jitter to prevent thundering herd
				jitter := secureJitter(baseDelay)
				delay = baseDelay + jitter
				if os.Getenv("DEBUG") != "" {
					log.Printf("[DEBUG] Retry attempt %d: using exponential backoff with jitter (base: %v, jitter: %v, total: %v)",
						attempt, baseDelay, jitter, delay)
				}
			}

			time.Sleep(delay)
		}

		data, resp, err := downloadWithRetryAndResponse(url, config)
		if err == nil {
			return data, nil
		}

		lastErr = err

		// Parse Retry-After header if present
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				retryAfterDuration = parseRetryAfter(retryAfter)
				if os.Getenv("DEBUG") != "" {
					if retryAfterDuration > 0 {
						if retryAfterDuration >= HFMaxRetryAfterDelay {
							log.Printf("[DEBUG] Retry-After header detected: %s (delay: %v, at or exceeding max allowed: %v)",
								retryAfter, retryAfterDuration, HFMaxRetryAfterDelay)
						} else {
							log.Printf("[DEBUG] Retry-After header detected: %s (parsed delay: %v)",
								retryAfter, retryAfterDuration)
						}
					} else {
						log.Printf("[DEBUG] Retry-After header detected but could not be parsed: %s", retryAfter)
					}
				}
			}
		}

		// Don't retry on certain errors
		if isNonRetryableError(err) {
			break
		}
	}

	return nil, lastErr
}

// downloadWithRetryAndResponse performs a single download attempt and returns the response.
// Unlike a simple download function, this returns the HTTP response alongside the data
// to allow the caller to inspect response headers (e.g., Retry-After header for rate limiting).
func downloadWithRetryAndResponse(url string, config *HFConfig) ([]byte, *http.Response, error) {
	// Create a context with timeout for this specific request
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create request")
	}

	// Set headers
	req.Header.Set("User-Agent", fmt.Sprintf("pure-tokenizers/%s", GetLibraryVersion()))
	if config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+config.Token)
	}

	// Use the shared HTTP client with connection pooling
	client := getHFHTTPClient(config)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check status code
	switch resp.StatusCode {
	case http.StatusOK:
		// Success
	case http.StatusUnauthorized:
		return nil, resp, errors.New("authentication required: please set HF_TOKEN environment variable or use WithHFToken()")
	case http.StatusForbidden:
		return nil, resp, errors.New("access forbidden: token may be invalid or model may be gated")
	case http.StatusNotFound:
		return nil, resp, errors.New("model or tokenizer.json not found")
	case http.StatusTooManyRequests:
		// Return with response so caller can parse Retry-After
		return nil, resp, errors.New("rate limited: too many requests")
	default:
		return nil, resp, errors.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Validate file size before downloading
	maxSize := config.MaxTokenizerSize
	if maxSize == 0 {
		maxSize = getEnvInt64("HF_MAX_TOKENIZER_SIZE", DefaultMaxTokenizerSize)
	}

	if maxSize > 0 && resp.ContentLength > 0 {
		if resp.ContentLength > maxSize {
			return nil, resp, errors.Errorf("tokenizer file too large: %d bytes exceeds maximum %d bytes", resp.ContentLength, maxSize)
		}

		// Log warning in DEBUG mode if file is large (>100MB)
		if os.Getenv("DEBUG") != "" && resp.ContentLength > 100*1024*1024 {
			log.Printf("[DEBUG] Downloading large tokenizer file: %d bytes (%.1f MB)",
				resp.ContentLength, float64(resp.ContentLength)/(1024*1024))
		}
	}

	// Read response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, errors.Wrap(err, "failed to read response")
	}

	// Validate it's valid JSON
	var validateJSON map[string]interface{}
	if err := json.Unmarshal(data, &validateJSON); err != nil {
		return nil, resp, errors.Wrap(err, "invalid tokenizer.json format")
	}

	return data, resp, nil
}

// validateModelID checks if the model ID is valid
func validateModelID(modelID string) error {
	// Empty model ID is handled separately in FromHuggingFace
	if modelID == "" {
		return nil
	}

	// Validate format: must be either "repo_name" or "owner/repo_name"
	parts := strings.Split(modelID, "/")
	if len(parts) > 2 {
		return errors.New("model ID must be in format 'owner/repo_name' or just 'repo_name'")
	}

	// If format is owner/repo_name, validate owner
	if len(parts) == 2 {
		owner := parts[0]
		if owner == "" {
			return errors.New("owner cannot be empty")
		}
		if owner == "." || owner == ".." {
			return errors.New("owner cannot be '.' or '..'")
		}
		// Owner follows same rules as repo_name
		if len(owner) > 96 {
			return errors.New("owner cannot exceed 96 characters")
		}
		if !isValidRepoName(owner) {
			return errors.New("owner contains invalid characters (must match [\\w\\-.]{1,96})")
		}
	}

	// Validate repo_name (last part)
	repoName := parts[len(parts)-1]
	if repoName == "" {
		return errors.New("repo_name cannot be empty")
	}
	if repoName == "." || repoName == ".." {
		return errors.New("repo_name cannot be '.' or '..'")
	}
	if len(repoName) > 96 {
		return errors.Errorf("repo_name cannot exceed 96 characters (got %d)", len(repoName))
	}
	if !isValidRepoName(repoName) {
		return errors.New("repo_name contains invalid characters (must match [\\w\\-.]{1,96})")
	}

	return nil
}

func validateHFRevision(revision string) error {
	trimmed := strings.TrimSpace(revision)
	if trimmed == "" {
		return errors.New("revision cannot be empty")
	}
	if len(trimmed) > 128 {
		return errors.Errorf("revision cannot exceed 128 characters (got %d)", len(trimmed))
	}
	if strings.Contains(trimmed, `\`) {
		return errors.New("revision cannot contain backslashes")
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasSuffix(trimmed, "/") {
		return errors.New("revision cannot start or end with '/'")
	}
	if strings.Contains(trimmed, ":") {
		return errors.New("revision cannot contain ':'")
	}

	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if part == "" {
			return errors.New("revision contains empty path segment")
		}
		if part == "." || part == ".." {
			return errors.New("revision contains forbidden path segment")
		}
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') ||
				(c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') ||
				c == '_' || c == '-' || c == '.') {
				return errors.New("revision contains invalid characters")
			}
		}
	}
	return nil
}

// isValidRepoName checks if a repo/owner name matches HuggingFace's pattern [\w\-.]{1,96}
func isValidRepoName(name string) bool {
	if len(name) == 0 || len(name) > 96 {
		return false
	}
	for _, c := range name {
		// \w matches [a-zA-Z0-9_], plus we allow - and .
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// getHFCachePath returns the cache path for a HuggingFace tokenizer
func getHFCachePath(customCacheDir, modelID, revision string) string {
	var cacheDir string
	if customCacheDir != "" {
		cacheDir = customCacheDir
	} else {
		cacheDir = getHFCacheDir()
	}

	// Sanitize model ID for filesystem
	sanitizedModelID := strings.ReplaceAll(modelID, "/", "--")
	safeRevision := strings.TrimSpace(revision)
	if safeRevision == "" || validateHFRevision(safeRevision) != nil {
		safeRevision = HFDefaultRevision
	}

	return filepath.Join(cacheDir, "models", sanitizedModelID, safeRevision, "tokenizer.json")
}

// getHFCacheDir returns the default HuggingFace cache directory
func getHFCacheDir() string {
	// Check HF environment variables first
	if hfHome := os.Getenv("HF_HOME"); hfHome != "" {
		return filepath.Join(hfHome, "tokenizers")
	}
	if hfCache := os.Getenv("HF_HUB_CACHE"); hfCache != "" {
		return filepath.Join(hfCache, "..", "tokenizers")
	}

	// Use our standard cache directory with HF subdirectory
	baseCache := getCacheDir()
	return filepath.Join(baseCache, "hf")
}

// saveToHFCache saves the tokenizer data to the cache with atomic write
func saveToHFCache(path string, data []byte) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}

	// Use atomic write to prevent race conditions
	tempPath := path + ".tmp" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return errors.Wrap(err, "failed to write cache file")
	}

	// Atomic rename
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath) // Clean up on failure
		return errors.Wrap(err, "failed to save cache file")
	}

	return nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// isNonRetryableError checks if an error should not be retried
func isNonRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "authentication required") ||
		strings.Contains(errStr, "access forbidden") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "invalid")
}

// GetHFCacheInfo returns information about the HuggingFace cache for a model
func GetHFCacheInfo(modelID string) (map[string]interface{}, error) {
	if err := validateModelID(modelID); err != nil {
		return nil, errors.Wrap(err, "invalid model ID")
	}

	info := make(map[string]interface{})
	info["model_id"] = modelID

	// Check default cache
	defaultPath := getHFCachePath("", modelID, HFDefaultRevision)
	info["default_cache_path"] = defaultPath
	info["is_cached"] = fileExists(defaultPath)

	if fileExists(defaultPath) {
		if stat, err := os.Stat(defaultPath); err == nil {
			info["cache_size"] = stat.Size()
			info["cache_modified"] = stat.ModTime()
		}
	}

	return info, nil
}

// ClearHFModelCache clears the cache for a specific model
func ClearHFModelCache(modelID string) error {
	if err := validateModelID(modelID); err != nil {
		return errors.Wrap(err, "invalid model ID")
	}

	cacheDir := getHFCacheDir()
	sanitizedModelID := strings.ReplaceAll(modelID, "/", "--")
	modelCacheDir := filepath.Join(cacheDir, "models", sanitizedModelID)

	if _, err := os.Stat(modelCacheDir); os.IsNotExist(err) {
		return nil // Already doesn't exist
	}

	return os.RemoveAll(modelCacheDir)
}

// ClearHFCache clears all HuggingFace tokenizer cache
func ClearHFCache() error {
	cacheDir := getHFCacheDir()
	modelsDir := filepath.Join(cacheDir, "models")

	if _, err := os.Stat(modelsDir); os.IsNotExist(err) {
		return nil // Already doesn't exist
	}

	return os.RemoveAll(modelsDir)
}

// ClearHFCachePattern clears cache entries matching a glob pattern.
// The pattern is matched against model IDs (e.g., "bert-*", "huggingface/*").
// Patterns use standard glob syntax: * matches any sequence, ? matches any single character.
//
// Examples:
//   - "bert-*" matches all BERT model variants
//   - "huggingface/*" matches all models from the huggingface organization
//   - "*/bert-*" matches BERT models from any organization
//
// For security, patterns containing ".." in path segments or absolute paths are rejected.
// Returns the number of cache entries cleared and any error encountered.
func ClearHFCachePattern(pattern string) (int, error) {
	// Security: prevent directory traversal attempts
	// Check if pattern is an absolute path
	if filepath.IsAbs(pattern) {
		return 0, errors.New("invalid pattern: absolute paths not allowed")
	}

	// Check each path component for ".." to prevent directory traversal
	// This prevents both "../etc" and valid-looking patterns like "bert-..base"
	parts := strings.Split(pattern, "/")
	for _, part := range parts {
		if part == ".." {
			return 0, errors.New("invalid pattern: directory traversal not allowed")
		}
	}

	// Additional check: patterns containing ".." anywhere are suspicious
	// This catches edge cases like "bert-base-..cased" which are unlikely to be legitimate
	if strings.Contains(pattern, "..") {
		return 0, errors.New("invalid pattern: patterns containing '..' are not allowed")
	}

	// Validate pattern syntax upfront for faster failure
	if _, err := filepath.Match(pattern, ""); err != nil {
		return 0, errors.Wrapf(err, "invalid glob pattern: %s", pattern)
	}

	cacheDir := getHFCacheDir()
	modelsDir := filepath.Join(cacheDir, "models")

	if _, err := os.Stat(modelsDir); os.IsNotExist(err) {
		return 0, nil // Cache directory doesn't exist
	}

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read cache directory")
	}

	cleared := 0
	var errs []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Convert sanitized directory name back to model ID (-- → /)
		modelID := strings.ReplaceAll(entry.Name(), "--", "/")

		// Check if model ID matches the glob pattern
		// Pattern already validated above, so err should never occur here
		matched, _ := filepath.Match(pattern, modelID)

		if matched {
			modelCacheDir := filepath.Join(modelsDir, entry.Name())
			if err := os.RemoveAll(modelCacheDir); err != nil {
				errs = append(errs, fmt.Sprintf("failed to clear %s: %v", modelID, err))
			} else {
				cleared++
			}
		}
	}

	if len(errs) > 0 {
		// Format error messages individually for better readability
		errMsg := fmt.Sprintf("cleared %d entries with %d errors:\n", cleared, len(errs))
		for i, e := range errs {
			errMsg += fmt.Sprintf("  %d. %s\n", i+1, e)
		}
		return cleared, errors.New(strings.TrimSpace(errMsg))
	}

	return cleared, nil
}

// parseRetryAfter parses the Retry-After header value.
// It can be either a delay in seconds or an HTTP date.
// The returned duration is capped at HFMaxRetryAfterDelay to prevent excessive waits.
func parseRetryAfter(value string) time.Duration {
	var duration time.Duration

	// First, try to parse as seconds
	if seconds, err := strconv.Atoi(value); err == nil {
		duration = time.Duration(seconds) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		// Try to parse as HTTP date (RFC1123)
		// Calculate duration from now
		duration = time.Until(t)
		if duration < 0 {
			// If the time is in the past, don't wait
			return 0
		}
	} else {
		// If we can't parse it, return 0 (fallback to exponential backoff)
		return 0
	}

	// Cap the delay to prevent excessive waits
	if duration > HFMaxRetryAfterDelay {
		return HFMaxRetryAfterDelay
	}
	return duration
}

func secureJitter(baseDelay time.Duration) time.Duration {
	maxJitter := baseDelay / 4
	if maxJitter <= 0 {
		return 0
	}
	limit := big.NewInt(maxJitter.Nanoseconds() + 1)
	n, err := cryptorand.Int(cryptorand.Reader, limit)
	if err != nil {
		// Fallback mixes a monotonic counter with current time to preserve retry variance.
		nonce := jitterNonce.Add(1)
		now := time.Now().UnixNano()
		mixed := now ^ nonce ^ (nonce << 13)
		if mixed < 0 {
			mixed = -mixed
		}
		return time.Duration(mixed % limit.Int64())
	}
	return time.Duration(n.Int64())
}

// checkHFHubCache checks if tokenizer exists in the standard HuggingFace hub cache
func checkHFHubCache(modelID, revision string) ([]byte, error) {
	if revision != "" {
		if err := validateHFRevision(revision); err != nil {
			return nil, errors.Wrap(err, "invalid HuggingFace revision")
		}
	}

	// Get HuggingFace hub cache directory
	hubCacheDir := getHFHubCacheDir()
	if hubCacheDir == "" {
		return nil, errors.New("HuggingFace hub cache directory not found")
	}

	// Convert model ID to cache format
	// HF uses "models--owner--name" format
	sanitizedModelID := "models--" + strings.ReplaceAll(modelID, "/", "--")

	// Check for snapshot directory
	snapshotDir := filepath.Join(hubCacheDir, sanitizedModelID, "snapshots")
	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		return nil, errors.Errorf("model not found in HF hub cache: %s", modelID)
	}

	// Look for the specific revision or find the latest
	var tokenizerPath string

	// First try to find by revision hash
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read snapshots directory")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check if this snapshot has tokenizer.json
		candidatePath := filepath.Join(snapshotDir, entry.Name(), "tokenizer.json")
		if fileExists(candidatePath) {
			// If we're looking for a specific revision, check refs
			if revision != "main" && revision != "" {
				// Check if this snapshot matches the revision
				refPath := filepath.Join(hubCacheDir, sanitizedModelID, "refs", revision)
				// #nosec G304 -- refPath is built from a validated revision under the HF cache root.
				if refData, err := os.ReadFile(refPath); err == nil {
					refHash := strings.TrimSpace(string(refData))
					if entry.Name() == refHash {
						tokenizerPath = candidatePath
						break
					}
				}
			} else {
				// For main/default, use the most recent snapshot
				tokenizerPath = candidatePath
			}
		}
	}

	if tokenizerPath == "" {
		return nil, errors.Errorf("tokenizer.json not found in HF hub cache for %s", modelID)
	}

	// Read and validate the tokenizer
	data, err := os.ReadFile(tokenizerPath) // #nosec G304 -- tokenizerPath is discovered from entries under the HF cache snapshots directory.
	if err != nil {
		return nil, errors.Wrap(err, "failed to read tokenizer from HF hub cache")
	}

	// Validate it's valid JSON
	var validateJSON map[string]interface{}
	if err := json.Unmarshal(data, &validateJSON); err != nil {
		return nil, errors.Wrap(err, "invalid tokenizer.json format in HF hub cache")
	}

	return data, nil
}

// getHFHubCacheDir returns the standard HuggingFace hub cache directory
func getHFHubCacheDir() string {
	// Check environment variables in order of priority
	if hfCache := os.Getenv("HF_HUB_CACHE"); hfCache != "" {
		return hfCache
	}
	if hfHome := os.Getenv("HF_HOME"); hfHome != "" {
		return filepath.Join(hfHome, "hub")
	}
	// Default locations
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "huggingface", "hub")
	}
	return ""
}

// loadFromCacheWithValidation loads tokenizer from cache with optional TTL validation
func loadFromCacheWithValidation(path string, ttl time.Duration) ([]byte, error) {
	// Single syscall for both existence and modtime check
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheNotFound
		}
		return nil, errors.Wrap(err, "failed to stat cache file")
	}

	// Check if it's a directory (defensive check; shouldn't occur in normal operation
	// as cache files are created via os.WriteFile, but could indicate cache corruption).
	if info.IsDir() {
		return nil, errors.New("cache path is a directory")
	}

	// Check if cache is still valid based on TTL
	if ttl > 0 && time.Since(info.ModTime()) > ttl {
		return nil, errors.New("cache expired")
	}

	// Read file (small race window remains between Stat and ReadFile, but acceptable
	// for cache scenarios; full elimination would require OS-specific file locking).
	data, err := os.ReadFile(path) // #nosec G304 -- path points to a cache file from trusted internal cache path construction.
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cache file")
	}

	// Validate JSON format
	var validateJSON map[string]interface{}
	if err := json.Unmarshal(data, &validateJSON); err != nil {
		return nil, errors.Wrap(err, "invalid cached tokenizer format")
	}

	return data, nil
}

// WithHFUseLocalCache enables or disables checking the HuggingFace hub cache
func WithHFUseLocalCache(useCache bool) TokenizerOption {
	return func(t *Tokenizer) error {
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.UseLocalCache = useCache
		return nil
	}
}

// WithHFCacheTTL sets the cache time-to-live for cached tokenizers
func WithHFCacheTTL(ttl time.Duration) TokenizerOption {
	return func(t *Tokenizer) error {
		if ttl < 0 {
			return errors.New("cache TTL must be non-negative")
		}
		if t.hfConfig == nil {
			t.hfConfig = &HFConfig{}
		}
		t.hfConfig.CacheTTL = ttl
		return nil
	}
}
