package chroma

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const envLibPath = "CHROMA_LIB_PATH"

type getenvFunc func(string) string
type statFunc func(string) (os.FileInfo, error)
type absPathFunc func(string) (string, error)

type libraryCandidate struct {
	path   string
	source string
}

type libraryLoadPlan struct {
	goos         string
	configured   string
	configSource string
	candidates   []libraryCandidate
	warnings     []string
}

type candidateSet struct {
	ordered []libraryCandidate
	seen    map[string]struct{}
}

func newCandidateSet(capacity int) candidateSet {
	return candidateSet{
		ordered: make([]libraryCandidate, 0, capacity),
		seen:    make(map[string]struct{}, capacity),
	}
}

func (set *candidateSet) add(path string, source string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if _, ok := set.seen[path]; ok {
		return
	}
	set.seen[path] = struct{}{}
	set.ordered = append(set.ordered, libraryCandidate{
		path:   path,
		source: source,
	})
}

func (set *candidateSet) candidates() []libraryCandidate {
	return set.ordered
}

func resolveLibraryLoadPlan(path string, goos string, getenv getenvFunc, stat statFunc) (libraryLoadPlan, error) {
	return resolveLibraryLoadPlanWithAbs(path, goos, getenv, stat, filepath.Abs)
}

func resolveLibraryLoadPlanWithAbs(path string, goos string, getenv getenvFunc, stat statFunc, abs absPathFunc) (libraryLoadPlan, error) {
	configuredPath, source := resolveConfiguredLibraryPath(path, getenv)
	if configuredPath == "" {
		return libraryLoadPlan{}, errors.Errorf(
			"library path not specified; pass Init(libPath) or set %s (expected %q on %s)",
			envLibPath,
			defaultLibraryFilename(goos),
			goos,
		)
	}

	candidates, candidateWarnings := buildLibraryPathCandidates(configuredPath, goos, stat)
	candidates, absWarnings := appendAbsolutePathCandidates(candidates, goos, abs)
	if len(candidates) == 0 {
		return libraryLoadPlan{}, errors.Errorf("library path %q resolved to no load candidates", configuredPath)
	}

	warnings := append([]string{}, candidateWarnings...)
	warnings = append(warnings, absWarnings...)

	return libraryLoadPlan{
		goos:         goos,
		configured:   configuredPath,
		configSource: source,
		candidates:   candidates,
		warnings:     warnings,
	}, nil
}

func resolveConfiguredLibraryPath(path string, getenv getenvFunc) (string, string) {
	configuredPath := strings.TrimSpace(path)
	if configuredPath != "" {
		return configuredPath, "Init(libPath)"
	}

	if getenv == nil {
		return "", ""
	}

	configuredPath = strings.TrimSpace(getenv(envLibPath))
	if configuredPath != "" {
		return configuredPath, envLibPath
	}

	return "", ""
}

func buildLibraryPathCandidates(path string, goos string, stat statFunc) ([]libraryCandidate, []string) {
	normalized := normalizePathSeparators(path, goos)
	extension := libraryFileExtension(goos)
	defaultName := defaultLibraryFilename(goos)

	candidates := newCandidateSet(6)
	warnings := make([]string, 0)

	isDirectory, directoryWarning := looksLikeDirectoryPath(normalized, stat)
	if directoryWarning != "" {
		warnings = append(warnings, directoryWarning)
	}
	if isDirectory {
		candidates.add(joinPathForOS(normalized, defaultName, goos), "derived:directory-default-name")
		return candidates.candidates(), warnings
	}

	candidates.add(normalized, "configured")

	if pathExtForOS(normalized) == "" {
		candidates.add(normalized+extension, "derived:added-extension")

		if goos != "windows" {
			dir, file := splitPathNormalized(normalized)
			if file != "" && !strings.HasPrefix(file, "lib") {
				candidates.add(joinPathForOS(dir, "lib"+file+extension, goos), "derived:added-lib-prefix")
			}
		}
	}

	return candidates.candidates(), warnings
}

func appendAbsolutePathCandidates(candidates []libraryCandidate, goos string, abs absPathFunc) ([]libraryCandidate, []string) {
	withAbsolute := newCandidateSet(len(candidates) * 2)

	for _, candidate := range candidates {
		withAbsolute.add(candidate.path, candidate.source)
	}

	warnings := make([]string, 0)
	for _, candidate := range candidates {
		if !strings.ContainsAny(candidate.path, `/\`) {
			continue
		}
		if isAbsolutePathForOS(candidate.path, goos) {
			continue
		}
		if abs == nil {
			warnings = append(warnings, fmt.Sprintf("skipped absolute fallback for [%s] %q: abs resolver unavailable", candidate.source, candidate.path))
			continue
		}

		absPath, err := abs(candidate.path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped absolute fallback for [%s] %q: %v", candidate.source, candidate.path, err))
			continue
		}

		if goos == "windows" {
			absPath = strings.ReplaceAll(absPath, "/", `\`)
		} else {
			absPath = strings.ReplaceAll(absPath, `\`, "/")
		}
		withAbsolute.add(absPath, "derived:absolute-fallback")
	}

	return withAbsolute.candidates(), warnings
}

func formatLoadAttempt(candidate libraryCandidate, err error) string {
	if err == nil {
		return fmt.Sprintf("[%s] %s (loader returned nil handle; unexpected)", candidate.source, candidate.path)
	}
	return fmt.Sprintf("[%s] %s (%v)", candidate.source, candidate.path, err)
}

func formatLibraryLoadError(plan libraryLoadPlan, attempts []string) error {
	candidateDescriptions := make([]string, 0, len(plan.candidates))
	for _, candidate := range plan.candidates {
		candidateDescriptions = append(candidateDescriptions, fmt.Sprintf("[%s] %s", candidate.source, candidate.path))
	}

	message := fmt.Sprintf(
		"failed to load Chroma library on %s using %s=%q; attempted paths: [%s]; expected extension: %q (default filename: %q); loader errors: %s",
		plan.goos,
		plan.configSource,
		plan.configured,
		strings.Join(candidateDescriptions, ", "),
		libraryFileExtension(plan.goos),
		defaultLibraryFilename(plan.goos),
		strings.Join(attempts, "; "),
	)
	if len(plan.warnings) > 0 {
		message = fmt.Sprintf("%s; path resolution warnings: %s", message, strings.Join(plan.warnings, "; "))
	}

	return errors.New(message)
}

func defaultLibraryFilename(goos string) string {
	switch goos {
	case "windows":
		return "chroma_shim.dll"
	case "darwin":
		return "libchroma_shim.dylib"
	default:
		return "libchroma_shim.so"
	}
}

func libraryFileExtension(goos string) string {
	switch goos {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	default:
		return ".so"
	}
}

func normalizePathSeparators(path string, goos string) string {
	if goos == "windows" {
		return strings.ReplaceAll(path, "/", `\`)
	}
	return strings.ReplaceAll(path, `\`, "/")
}

func looksLikeDirectoryPath(path string, stat statFunc) (bool, string) {
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, `\`) {
		if stat == nil {
			return true, ""
		}

		info, err := stat(path)
		if err == nil {
			if info.IsDir() {
				return true, ""
			}
			return true, fmt.Sprintf("path %q ends with a directory separator but stat reports a non-directory", path)
		}

		return true, fmt.Sprintf("path %q ends with a directory separator but stat failed: %v", path, err)
	}
	if stat == nil {
		return false, ""
	}

	info, err := stat(path)
	if err == nil {
		return info.IsDir(), ""
	}
	if os.IsNotExist(err) {
		return false, ""
	}

	return false, fmt.Sprintf("stat(%q) failed while inferring directory path: %v", path, err)
}

func pathExtForOS(path string) string {
	_, file := splitPathNormalized(path)
	lastDot := strings.LastIndex(file, ".")
	if lastDot <= 0 || lastDot == len(file)-1 {
		return ""
	}
	return strings.ToLower(file[lastDot:])
}

func splitPathNormalized(path string) (string, string) {
	normalized := strings.ReplaceAll(path, `\`, "/")
	lastSep := strings.LastIndex(normalized, "/")
	if lastSep == -1 {
		return "", normalized
	}

	dir := normalized[:lastSep]
	file := normalized[lastSep+1:]
	return dir, file
}

func joinPathForOS(dir string, file string, goos string) string {
	if dir == "" || dir == "." {
		return file
	}

	if goos == "windows" {
		normalizedDir := strings.ReplaceAll(dir, "/", `\`)
		if normalizedDir == `\` {
			return `\` + file
		}
		cleanDir := strings.TrimRight(normalizedDir, `\`)
		if cleanDir == "" {
			return file
		}
		return cleanDir + `\` + file
	}

	normalizedDir := strings.ReplaceAll(dir, `\`, "/")
	if normalizedDir == "/" {
		return "/" + file
	}

	cleanDir := strings.TrimRight(normalizedDir, "/")
	if cleanDir == "" {
		return file
	}

	return cleanDir + "/" + file
}

func isAbsolutePathForOS(path string, goos string) bool {
	if goos != "windows" {
		return strings.HasPrefix(path, "/")
	}

	if strings.HasPrefix(path, `\\`) {
		return true
	}

	if len(path) < 3 {
		return false
	}

	drive := path[0]
	hasDriveLetter := (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
	if !hasDriveLetter || path[1] != ':' {
		return false
	}

	return path[2] == '\\' || path[2] == '/'
}
