package tokenizers

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ebitengine/purego"
)

const (
	// GitHubRepo is the fixed GitHub repository used for fallback downloads.
	GitHubRepo = "amikos-tech/pure-tokenizers"
	// ReleasesBaseURL is the fixed public endpoint for release artifacts.
	ReleasesBaseURL = "https://releases.amikos.tech"
	// ReleasesProject is the fixed project path under the releases domain.
	ReleasesProject = "pure-tokenizers"
	DefaultTag      = "latest"
	DownloadTimeout = 30 * time.Second
	// MaxSharedLibrarySize limits extracted shared library size to 200MB.
	// This prevents decompression bomb style archives from exhausting disk/memory.
	MaxSharedLibrarySize = 200 * 1024 * 1024
	// gitHubReleasesPageSize is the number of releases fetched per API page.
	gitHubReleasesPageSize = 100
	// gitHubReleasesMaxPages bounds pagination while still covering large histories.
	gitHubReleasesMaxPages = 20
	// apiVer is the GitHub REST API version sent via X-GitHub-Api-Version.
	apiVer = "2022-11-28"
)

var (
	warnVersionsFallbackOnce   sync.Once
	warnIgnoredLegacyRepoEnv   sync.Once
	libraryABIVerifiedMu       sync.RWMutex
	libraryABIVerifiedByPath   = make(map[string]string)
	errChecksumAssetNotFound   = errors.New("checksum for asset not found")
	errChecksumManifestInvalid = errors.New("invalid checksum manifest")
	releasesBaseHostname       = func() string {
		parsed, err := url.Parse(ReleasesBaseURL)
		if err != nil {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	}()
	allowedChecksumsHosts = func() map[string]struct{} {
		hosts := map[string]struct{}{
			"objects.githubusercontent.com": {},
		}
		if releasesBaseHostname != "" {
			hosts[releasesBaseHostname] = struct{}{}
		}
		return hosts
	}()
)

// getPlatformAssetName returns the expected asset name for the current platform
func getPlatformAssetName() string {
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	default:
		arch = runtime.GOARCH
	}

	var platform string
	switch runtime.GOOS {
	case "darwin":
		platform = "apple-darwin"
	case "linux":
		if isMusl() {
			platform = "unknown-linux-musl"
		} else {
			platform = "unknown-linux-gnu"
		}
	case "windows":
		platform = "pc-windows-msvc"
	default:
		platform = runtime.GOOS
	}

	return fmt.Sprintf("libtokenizers-%s-%s.tar.gz", arch, platform)
}

// DownloadLibraryFromGitHub downloads the platform-specific library.
// Legacy name is kept for API compatibility. Downloads use releases.amikos.tech first, then GitHub fallback.
func DownloadLibraryFromGitHub(destPath string) error {
	version := getVersionTag()
	return DownloadLibraryFromGitHubWithVersion(destPath, version)
}

// downloadFile downloads a file from the given URL to the destination path
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: DownloadTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}
	setRequestHeaders(req, "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download from %s: %w", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d: %s (%s)", resp.StatusCode, resp.Status, url)
	}

	out, err := os.Create(dest) // #nosec G304 -- destination path is intentionally caller-controlled output location.
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", dest, err)
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		_ = out.Close()
		if removeErr := os.Remove(dest); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("failed to write file %s: %w (also failed to remove partial file: %v)", dest, err, removeErr)
		}
		return fmt.Errorf("failed to write file %s: %w", dest, err)
	}

	return nil
}

func setRequestHeaders(req *http.Request, accept string) {
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "pure-tokenizers-downloader")

	// Only attach GitHub-specific headers/tokens when talking to GitHub APIs.
	if strings.EqualFold(req.URL.Hostname(), "api.github.com") {
		req.Header.Set("X-GitHub-Api-Version", apiVer)
		if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		} else if tok := os.Getenv("GH_TOKEN"); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
}

func downloadJSON(url string, out any) error {
	client := &http.Client{Timeout: DownloadTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}
	setRequestHeaders(req, "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JSON from %s: %w", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d: %s (%s)", resp.StatusCode, resp.Status, url)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode JSON from %s: %w", url, err)
	}
	return nil
}

// verifyChecksum verifies the SHA256 checksum of the downloaded file
func verifyChecksum(filePath, checksumData string) error {
	expectedChecksum := strings.TrimSpace(checksumData)
	if expectedChecksum == "" {
		return fmt.Errorf("checksum data is empty")
	}

	// Handle format like "abc123  filename.tar.gz"
	if parts := strings.Fields(expectedChecksum); len(parts) >= 1 {
		expectedChecksum = parts[0]
	}
	if expectedChecksum == "" {
		return fmt.Errorf("checksum data is empty")
	}

	// Calculate actual checksum
	file, err := os.Open(filePath) // #nosec G304 -- filePath points to a downloaded temp artifact selected by internal flow.
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

// extractLibrary extracts the shared library from the tar.gz archive
func extractLibrary(archivePath, destPath string) error {
	file, err := os.Open(archivePath) // #nosec G304 -- archivePath is generated internally in a controlled temp directory.
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		_ = gzr.Close()
	}()

	tr := tar.NewReader(gzr)
	libraryName := getLibraryName()

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Look for the library file (could be in subdirectories)
		if strings.HasSuffix(header.Name, libraryName) {
			// Extract this file to the destination
			outFile, err := os.Create(destPath) // #nosec G304 -- destPath is explicit caller-provided output path.
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer func() {
				_ = outFile.Close()
			}()

			if header.Size <= 0 || header.Size > MaxSharedLibrarySize {
				return fmt.Errorf(
					"archive entry %s has unsupported size %d (max %d)",
					header.Name,
					header.Size,
					MaxSharedLibrarySize,
				)
			}
			if _, err := io.CopyN(outFile, tr, header.Size); err != nil {
				return fmt.Errorf("failed to extract library: %w", err)
			}

			// Make the library executable
			// #nosec G302 -- shared libraries require execute permissions to be loadable.
			if err := os.Chmod(destPath, 0755); err != nil {
				return fmt.Errorf("failed to set library permissions: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("library file %s not found in archive", libraryName)
}

type releaseIndex struct {
	Version      string `json:"version"`
	ChecksumsURL string `json:"checksums_url"`
}

type gitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []gitHubAsset `json:"assets"`
}

type gitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func isAllowedChecksumsHost(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	if normalized == "" {
		return false
	}
	_, ok := allowedChecksumsHosts[normalized]
	return ok
}

func buildReleaseURL(parts ...string) string {
	base := ReleasesBaseURL
	pathParts := make([]string, 0, len(parts))
	for _, p := range parts {
		part := strings.Trim(p, "/")
		if part != "" {
			pathParts = append(pathParts, part)
		}
	}
	if len(pathParts) == 0 {
		return base
	}
	return fmt.Sprintf("%s/%s", base, strings.Join(pathParts, "/"))
}

func normalizeReleaseVersion(version string) string {
	v := strings.TrimSpace(version)
	switch {
	case v == "" || v == DefaultTag:
		return DefaultTag
	case strings.HasPrefix(v, "rust-v"):
		return v
	case strings.HasPrefix(v, "rust-"):
		suffix := strings.TrimPrefix(v, "rust-")
		if suffix == "" {
			return DefaultTag
		}
		if suffix[0] >= '0' && suffix[0] <= '9' {
			return "rust-v" + suffix
		}
		return v
	case strings.HasPrefix(v, "v"):
		return "rust-" + v
	default:
		return "rust-v" + v
	}
}

func fetchLatestReleaseIndex() (*releaseIndex, error) {
	url := buildReleaseURL(ReleasesProject, "latest.json")
	var idx releaseIndex
	if err := downloadJSON(url, &idx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(idx.Version) == "" {
		return nil, fmt.Errorf("latest.json at %s is missing version", url)
	}
	return &idx, nil
}

func fetchGitHubReleaseByTag(tag string) (*gitHubRelease, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", GitHubRepo, tag)

	var release gitHubRelease
	if err := downloadJSON(endpoint, &release); err != nil {
		return nil, fmt.Errorf("failed to fetch GitHub release %s: %w", tag, err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("GitHub release payload missing tag name for %s", tag)
	}
	return &release, nil
}

func fetchLatestGitHubRustRelease() (*gitHubRelease, error) {
	for page := 1; page <= gitHubReleasesMaxPages; page++ {
		endpoint := fmt.Sprintf(
			"https://api.github.com/repos/%s/releases?per_page=%d&page=%d",
			GitHubRepo,
			gitHubReleasesPageSize,
			page,
		)

		var releases []gitHubRelease
		if err := downloadJSON(endpoint, &releases); err != nil {
			return nil, fmt.Errorf("failed to fetch GitHub releases list page %d: %w", page, err)
		}

		// GitHub returns releases newest-first, so the first rust-v* tag encountered is the latest.
		for _, release := range releases {
			if strings.HasPrefix(strings.TrimSpace(release.TagName), "rust-v") {
				return &release, nil
			}
		}

		// No more pages to scan.
		if len(releases) < gitHubReleasesPageSize {
			break
		}
	}
	return nil, fmt.Errorf(
		"no rust-v* releases found in first %d GitHub release page(s) for repository %s",
		gitHubReleasesMaxPages,
		GitHubRepo,
	)
}

func resolveChecksumsURL(version string, idx *releaseIndex) (string, error) {
	if idx != nil {
		checksumsURL := strings.TrimSpace(idx.ChecksumsURL)
		if checksumsURL != "" {
			parsed, err := url.Parse(checksumsURL)
			if err != nil {
				return "", fmt.Errorf("invalid checksums_url %q: %w", checksumsURL, err)
			}
			if parsed.IsAbs() {
				if !strings.EqualFold(parsed.Scheme, "https") {
					return "", fmt.Errorf("invalid checksums_url scheme %q: only https is allowed", parsed.Scheme)
				}
				if !isAllowedChecksumsHost(parsed.Hostname()) {
					return "", fmt.Errorf("invalid checksums_url host %q: host is not in the allowlist", parsed.Hostname())
				}
				return checksumsURL, nil
			}
			return buildReleaseURL(checksumsURL), nil
		}
	}
	return buildReleaseURL(ReleasesProject, version, "SHA256SUMS"), nil
}

func libraryABIFingerprint(path string) (string, error) {
	info, err := os.Stat(path) // #nosec G703 -- path comes from controlled cache resolution and explicit user library overrides.
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()), nil
}

func isLibraryABIVerified(path string) bool {
	fingerprint, err := libraryABIFingerprint(path)
	if err != nil {
		return false
	}

	libraryABIVerifiedMu.RLock()
	cached, ok := libraryABIVerifiedByPath[path]
	libraryABIVerifiedMu.RUnlock()
	return ok && cached == fingerprint
}

func markLibraryABIVerified(path string) {
	fingerprint, err := libraryABIFingerprint(path)
	if err != nil {
		return
	}

	libraryABIVerifiedMu.Lock()
	libraryABIVerifiedByPath[path] = fingerprint
	libraryABIVerifiedMu.Unlock()
}

func clearLibraryABIVerified(path string) {
	libraryABIVerifiedMu.Lock()
	delete(libraryABIVerifiedByPath, path)
	libraryABIVerifiedMu.Unlock()
}

// getVersionTag returns the version tag to download
func getVersionTag() string {
	if tag := os.Getenv("TOKENIZERS_VERSION"); tag != "" {
		return tag
	}
	return DefaultTag
}

func warnIfIgnoredLegacyRepoEnvSet() {
	if os.Getenv("TOKENIZERS_GITHUB_REPO") == "" {
		return
	}
	warnIgnoredLegacyRepoEnv.Do(func() {
		_, _ = fmt.Fprintln(
			os.Stderr,
			"warning: TOKENIZERS_GITHUB_REPO is ignored; fallback repository is fixed to amikos-tech/pure-tokenizers",
		)
	})
}

// DownloadLibraryFromGitHubWithVersion downloads a specific version of the library.
// Legacy name is kept for API compatibility.
// Downloads are attempted from releases.amikos.tech first, then fallback to GitHub Releases.
func DownloadLibraryFromGitHubWithVersion(destPath, version string) error {
	warnIfIgnoredLegacyRepoEnvSet()

	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0750); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	primaryErr := downloadFromReleasesWithVersion(destPath, version)
	if primaryErr == nil {
		return nil
	}

	_, _ = fmt.Fprintf(
		os.Stderr,
		"warning: releases endpoint download failed, falling back to GitHub Releases (%v)\n",
		primaryErr,
	)

	if fallbackErr := downloadFromGitHubWithVersion(destPath, version); fallbackErr != nil {
		return fmt.Errorf("download failed from releases endpoint (%v) and GitHub fallback (%w)", primaryErr, fallbackErr)
	}
	return nil
}

func downloadFromReleasesWithVersion(destPath, version string) error {
	resolvedVersion := normalizeReleaseVersion(version)
	var idx *releaseIndex
	var err error
	if resolvedVersion == DefaultTag {
		idx, err = fetchLatestReleaseIndex()
		if err != nil {
			return fmt.Errorf("failed to fetch latest release metadata: %w", err)
		}
		resolvedVersion = normalizeReleaseVersion(idx.Version)
	}

	if resolvedVersion == DefaultTag {
		return fmt.Errorf("failed to resolve a concrete release version from %q", version)
	}

	checksumsURL, err := resolveChecksumsURL(resolvedVersion, idx)
	if err != nil {
		return fmt.Errorf("failed to resolve checksums URL: %w", err)
	}

	return downloadAndExtractLibraryFromReleases(resolvedVersion, checksumsURL, destPath)
}

func downloadFromGitHubWithVersion(destPath, version string) error {
	resolvedVersion := normalizeReleaseVersion(version)

	var release *gitHubRelease
	var err error
	if resolvedVersion == DefaultTag {
		release, err = fetchLatestGitHubRustRelease()
	} else {
		release, err = fetchGitHubReleaseByTag(resolvedVersion)
	}
	if err != nil {
		return err
	}

	return downloadAndExtractLibraryFromGitHub(release, destPath)
}

func downloadAndExtractLibraryFromReleases(version, checksumsURL, destPath string) error {
	assetName := getPlatformAssetName()
	project := ReleasesProject
	assetURL := buildReleaseURL(project, version, assetName)

	// Create temporary files for download
	tempDir, err := os.MkdirTemp("", "tokenizers-download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Clean up temp files
	}()

	tempAsset := filepath.Join(tempDir, assetName)
	tempChecksums := filepath.Join(tempDir, "SHA256SUMS")

	// Download the archive
	if err := downloadFile(assetURL, tempAsset); err != nil {
		return fmt.Errorf("failed to download asset: %w", err)
	}

	// Download and resolve checksums.
	if err := downloadFile(checksumsURL, tempChecksums); err != nil {
		return fmt.Errorf("failed to download checksums from %s: %w", checksumsURL, err)
	}

	checksumData, err := os.ReadFile(tempChecksums) // #nosec G304 -- tempChecksums is created in a controlled temp directory.
	if err != nil {
		return fmt.Errorf("failed to read checksums file: %w", err)
	}

	assetChecksum, err := checksumForAsset(string(checksumData), assetName)
	if err != nil {
		if !errors.Is(err, errChecksumAssetNotFound) {
			return fmt.Errorf("failed to parse checksum manifest from %s: %w", checksumsURL, err)
		}
		_, _ = fmt.Fprintf(
			os.Stderr,
			"warning: checksum entry for %s missing in SHA256SUMS from %s; falling back to per-asset .sha256\n",
			assetName,
			checksumsURL,
		)

		// Fallback to per-asset checksum files for compatibility.
		perAssetChecksumURL := buildReleaseURL(project, version, assetName+".sha256")
		tempPerAssetChecksum := filepath.Join(tempDir, assetName+".sha256")
		if dlErr := downloadFile(perAssetChecksumURL, tempPerAssetChecksum); dlErr != nil {
			return fmt.Errorf(
				"failed to resolve checksum for %s from SHA256SUMS and fallback .sha256: %w (fallback error: %v)",
				assetName,
				err,
				dlErr,
			)
		}
		perAssetChecksumData, readErr := os.ReadFile(tempPerAssetChecksum) // #nosec G304 -- temp checksum file path is controlled and local.
		if readErr != nil {
			return fmt.Errorf("failed to read fallback checksum file: %w", readErr)
		}
		assetChecksum = string(perAssetChecksumData)
	}

	if err := verifyChecksum(tempAsset, assetChecksum); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Extract the library from the tar.gz file
	if err := extractLibrary(tempAsset, destPath); err != nil {
		return fmt.Errorf("failed to extract library: %w", err)
	}

	return nil
}

func downloadAndExtractLibraryFromGitHub(release *gitHubRelease, destPath string) error {
	if release == nil {
		return fmt.Errorf("nil GitHub release payload")
	}

	assetName := getPlatformAssetName()
	var assetURL string
	var checksumsURL string
	var perAssetChecksumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			assetURL = asset.BrowserDownloadURL
		case "SHA256SUMS":
			checksumsURL = asset.BrowserDownloadURL
		case assetName + ".sha256":
			perAssetChecksumURL = asset.BrowserDownloadURL
		}
	}

	if assetURL == "" {
		return fmt.Errorf("asset %s not found in GitHub release %s", assetName, release.TagName)
	}

	tempDir, err := os.MkdirTemp("", "tokenizers-download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	tempAsset := filepath.Join(tempDir, assetName)
	if err := downloadFile(assetURL, tempAsset); err != nil {
		return fmt.Errorf("failed to download asset from GitHub release %s: %w", release.TagName, err)
	}

	var assetChecksum string
	if checksumsURL != "" {
		tempChecksums := filepath.Join(tempDir, "SHA256SUMS")
		if err := downloadFile(checksumsURL, tempChecksums); err != nil {
			return fmt.Errorf("failed to download SHA256SUMS from GitHub release %s: %w", release.TagName, err)
		}

		checksumData, err := os.ReadFile(tempChecksums) // #nosec G304 -- tempChecksums is created in a controlled temp directory.
		if err != nil {
			return fmt.Errorf("failed to read GitHub SHA256SUMS: %w", err)
		}

		assetChecksum, err = checksumForAsset(string(checksumData), assetName)
		if err != nil {
			if !errors.Is(err, errChecksumAssetNotFound) {
				return fmt.Errorf("failed to parse GitHub SHA256SUMS: %w", err)
			}
			_, _ = fmt.Fprintf(
				os.Stderr,
				"warning: checksum entry for %s missing in GitHub SHA256SUMS for release %s; falling back to per-asset .sha256\n",
				assetName,
				release.TagName,
			)
		}
	}

	if assetChecksum == "" {
		if perAssetChecksumURL == "" {
			return fmt.Errorf("no checksum asset found for %s in GitHub release %s", assetName, release.TagName)
		}
		tempPerAssetChecksum := filepath.Join(tempDir, assetName+".sha256")
		if err := downloadFile(perAssetChecksumURL, tempPerAssetChecksum); err != nil {
			return fmt.Errorf("failed to download per-asset checksum from GitHub release %s: %w", release.TagName, err)
		}
		perAssetChecksumData, err := os.ReadFile(tempPerAssetChecksum) // #nosec G304 -- temp checksum file path is controlled and local.
		if err != nil {
			return fmt.Errorf("failed to read per-asset checksum file: %w", err)
		}
		assetChecksum = string(perAssetChecksumData)
	}

	if err := verifyChecksum(tempAsset, assetChecksum); err != nil {
		return fmt.Errorf("checksum verification failed for GitHub release %s: %w", release.TagName, err)
	}
	if err := extractLibrary(tempAsset, destPath); err != nil {
		return fmt.Errorf("failed to extract library from GitHub release %s: %w", release.TagName, err)
	}
	return nil
}

func checksumForAsset(checksums, assetName string) (string, error) {
	hasEntries := false
	lines := strings.Split(checksums, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			return "", fmt.Errorf("%w: line %d has invalid format", errChecksumManifestInvalid, i+1)
		}

		sum := parts[0]
		if !looksLikeSHA256(sum) {
			return "", fmt.Errorf("%w: line %d has invalid sha256 %q", errChecksumManifestInvalid, i+1, sum)
		}

		hasEntries = true
		candidate := strings.TrimPrefix(parts[len(parts)-1], "*")
		if filepath.Base(candidate) == assetName {
			return sum, nil
		}
	}
	if !hasEntries {
		return "", fmt.Errorf("%w: checksum manifest is empty", errChecksumManifestInvalid)
	}
	return "", fmt.Errorf("%w: %s", errChecksumAssetNotFound, assetName)
}

func looksLikeSHA256(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// DownloadAndCacheLibrary downloads and caches the library for the current platform
func DownloadAndCacheLibrary() error {
	cacheDir := getCacheDir()
	cachedPath := filepath.Join(cacheDir, getLibraryName())

	// Check if already cached and valid
	if _, statErr := os.Stat(cachedPath); statErr == nil {
		// Verify ABI compatibility of cached library
		err := verifyLibraryABICompatibility(cachedPath)
		if err == nil {
			return nil
		}
		_, _ = fmt.Fprintf(
			os.Stderr,
			"warning: cached library at %s failed ABI compatibility check (%v); clearing cache and re-downloading\n",
			cachedPath,
			err,
		)
		// If ABI check fails, clear cache and re-download
		if clearErr := ClearLibraryCache(); clearErr != nil {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"warning: failed to clear cached library %s (%v); continuing with re-download attempt\n",
				cachedPath,
				clearErr,
			)
		}
	}

	if err := DownloadLibraryFromGitHub(cachedPath); err != nil {
		return err
	}

	// Ensure a freshly downloaded library is ABI/symbol compatible.
	if err := verifyLibraryABICompatibility(cachedPath); err != nil {
		if clearErr := ClearLibraryCache(); clearErr != nil {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"warning: failed to clear incompatible downloaded library %s (%v)\n",
				cachedPath,
				clearErr,
			)
		}
		return fmt.Errorf("downloaded library at %s failed ABI compatibility check: %w", cachedPath, err)
	}

	return nil
}

// DownloadAndCacheLibraryWithVersion downloads and caches a specific version of the library
func DownloadAndCacheLibraryWithVersion(version string) error {
	cacheDir := getCacheDir()
	cachedPath := filepath.Join(cacheDir, getLibraryName())

	if err := DownloadLibraryFromGitHubWithVersion(cachedPath, version); err != nil {
		return err
	}

	// Ensure a freshly downloaded versioned library is ABI/symbol compatible.
	if err := verifyLibraryABICompatibility(cachedPath); err != nil {
		if clearErr := ClearLibraryCache(); clearErr != nil {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"warning: failed to clear incompatible downloaded library %s (%v)\n",
				cachedPath,
				clearErr,
			)
		}
		return fmt.Errorf("downloaded library at %s failed ABI compatibility check: %w", cachedPath, err)
	}

	return nil
}

// GetCachedLibraryPath returns the path where the library would be cached
func GetCachedLibraryPath() string {
	cacheDir := getCacheDir()
	return filepath.Join(cacheDir, getLibraryName())
}

// ClearLibraryCache removes the cached library file
func ClearLibraryCache() error {
	cachedPath := GetCachedLibraryPath()
	clearLibraryABIVerified(cachedPath)
	if _, err := os.Stat(cachedPath); os.IsNotExist(err) {
		return nil // Already doesn't exist
	}
	return os.Remove(cachedPath)
}

// GetAvailableVersions fetches available versions from the primary releases endpoint,
// and falls back to GitHub releases when needed.
// The return type is kept for API compatibility, but current endpoint contracts expose only
// latest.json, so this intentionally returns at most one latest version.
func GetAvailableVersions() ([]string, error) {
	warnIfIgnoredLegacyRepoEnvSet()

	idx, err := fetchLatestReleaseIndex()
	if err == nil {
		version := normalizeReleaseVersion(idx.Version)
		if version == DefaultTag {
			return nil, fmt.Errorf("latest release metadata contains invalid version %q", idx.Version)
		}
		return []string{version}, nil
	}

	release, fallbackErr := fetchLatestGitHubRustRelease()
	if fallbackErr != nil {
		return nil, fmt.Errorf("failed to fetch latest versions from releases endpoint (%v) and GitHub fallback (%w)", err, fallbackErr)
	}

	warnVersionsFallbackOnce.Do(func() {
		_, _ = fmt.Fprintf(
			os.Stderr,
			"warning: versions endpoint unavailable, falling back to GitHub Releases (%v)\n",
			err,
		)
	})

	return []string{release.TagName}, nil
}

// IsLibraryCached checks if the library is already cached and loadable.
// ABI compatibility is validated when loading through LoadTokenizerLibrary/DownloadAndCacheLibrary.
func IsLibraryCached() bool {
	cachedPath := GetCachedLibraryPath()
	return isLibraryValid(cachedPath)
}

// verifyLibraryABICompatibility checks if a library file is ABI compatible with the current Go bindings
func verifyLibraryABICompatibility(libraryPath string) (err error) {
	if isLibraryABIVerified(libraryPath) {
		return nil
	}

	libh, err := loadLibrary(libraryPath)
	if err != nil {
		return fmt.Errorf("failed to load library for ABI compatibility check: %w", err)
	}
	defer func() {
		if closeErr := closeLibrary(libh); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close library after ABI check: %w", closeErr)
		}
	}()

	if err := verifyLibraryABICompatibilityHandle(libh); err != nil {
		return err
	}
	markLibraryABIVerified(libraryPath)
	return nil
}

func verifyLibraryABICompatibilityHandle(libh uintptr) error {
	requiredSymbols := []string{
		"from_file",
		"from_bytes",
		"encode",
		"encode_batch_pairs",
		"free_buffer",
		"free_tokenizer",
		"free_string",
		"decode",
		"vocab_size",
		"get_version",
	}

	for _, symbol := range requiredSymbols {
		if symbolErr := symbolExists(libh, symbol); symbolErr != nil {
			return fmt.Errorf("library is missing required symbol %q: %w", symbol, symbolErr)
		}
	}

	var getVersion func() string
	purego.RegisterLibFunc(&getVersion, libh, "get_version")

	versionStr := strings.TrimSpace(getVersion())
	if versionStr == "" {
		return fmt.Errorf("library returned empty version from get_version")
	}

	constraint, constraintErr := semver.NewConstraint(AbiCompatibilityConstraint)
	if constraintErr != nil {
		return fmt.Errorf("failed to parse ABI compatibility constraint %q: %w", AbiCompatibilityConstraint, constraintErr)
	}

	ver, versionErr := semver.NewVersion(versionStr)
	if versionErr != nil {
		return fmt.Errorf("invalid library version %q from get_version: %w", versionStr, versionErr)
	}
	if !constraint.Check(ver) {
		return fmt.Errorf(
			"library ABI version %s is not compatible with required constraint %s",
			versionStr,
			constraint.String(),
		)
	}

	return nil
}

// GetLibraryInfo returns information about the current library setup
func GetLibraryInfo() map[string]any {
	info := make(map[string]any)

	info["platform_asset_name"] = getPlatformAssetName()
	info["library_name"] = getLibraryName()
	info["cache_path"] = GetCachedLibraryPath()
	info["cache_dir"] = getCacheDir()
	info["is_cached"] = IsLibraryCached()
	info["releases_base_url"] = ReleasesBaseURL
	info["releases_project"] = ReleasesProject
	info["github_repo"] = GitHubRepo
	info["version"] = getVersionTag()

	// Check environment variables
	env := make(map[string]string)
	if path := os.Getenv("TOKENIZERS_LIB_PATH"); path != "" {
		env["TOKENIZERS_LIB_PATH"] = path
	}
	if version := os.Getenv("TOKENIZERS_VERSION"); version != "" {
		env["TOKENIZERS_VERSION"] = version
	}
	info["environment"] = env

	return info
}
