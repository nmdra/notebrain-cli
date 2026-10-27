package embedder

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// These pins match chroma-go/pkg/embeddings/default_ef's model download.
const modelArchiveURL = "https://chroma-onnx-models.s3.amazonaws.com/all-MiniLM-L6-v2/onnx.tar.gz"
const modelArchiveSHA256 = "913d7300ceae3b2dbc2c50d1de4baacab4be7b9380491c27fab7418616a16ec3"

// RepairModel downloads verified MiniLM artifacts that are missing from the local
// cache. It never removes an existing download lock or replaces healthy files.
func RepairModel(ctx context.Context) error {
	paths, err := defaultModelPaths()
	if err != nil {
		return fmt.Errorf("resolve model repair paths: %w", err)
	}
	return repairModel(ctx, paths, modelArchiveURL, modelArchiveSHA256)
}

func repairModel(ctx context.Context, paths modelPaths, url, checksum string) (result error) {
	availabilityErr := checkModelAvailability(paths, processIsRunning)
	if availabilityErr == nil {
		return nil
	}
	var availability *ModelAvailabilityError
	if !errors.As(availabilityErr, &availability) || availability.Reason != AvailabilityMissingArtifacts {
		return fmt.Errorf("refuse model repair: %w", availabilityErr)
	}
	if err := os.MkdirAll(paths.cacheDir, 0755); err != nil {
		return fmt.Errorf("create model cache: %w", err)
	}
	lock, err := os.OpenFile(paths.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("acquire model download lock: %w", err)
	}
	defer func() {
		// Keep the descriptor open until the identity check to protect replacement locks.
		owned, statErr := lock.Stat()
		current, pathErr := os.Lstat(paths.lockPath)
		if statErr == nil && pathErr == nil && os.SameFile(owned, current) {
			if removeErr := os.Remove(paths.lockPath); removeErr != nil {
				result = errors.Join(result, fmt.Errorf("remove model download lock: %w", removeErr))
			}
		}
		if closeErr := lock.Close(); closeErr != nil {
			result = errors.Join(result, fmt.Errorf("close model download lock: %w", closeErr))
		}
	}()
	if _, writeErr := fmt.Fprintf(lock, "%d", os.Getpid()); writeErr != nil {
		return fmt.Errorf("write model download lock: %w", writeErr)
	}
	if syncErr := lock.Sync(); syncErr != nil {
		return fmt.Errorf("sync model download lock: %w", syncErr)
	}
	stage, err := os.MkdirTemp(paths.cacheDir, ".repair-*")
	if err != nil {
		return fmt.Errorf("stage model repair: %w", err)
	}
	defer os.RemoveAll(stage)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create model request: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download model archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download model archive: unexpected HTTP status %s", resp.Status)
	}
	archive, err := os.Create(filepath.Join(stage, "archive.tar.gz"))
	if err != nil {
		return fmt.Errorf("stage archive: %w", err)
	}
	defer archive.Close()
	hash := sha256.New()
	if _, copyErr := io.Copy(io.MultiWriter(archive, hash), resp.Body); copyErr != nil {
		return fmt.Errorf("download archive: %w", copyErr)
	}
	if actual := fmt.Sprintf("%x", hash.Sum(nil)); actual != checksum {
		return fmt.Errorf("model archive checksum mismatch: expected %s, got %s", checksum, actual)
	}
	if _, seekErr := archive.Seek(0, io.SeekStart); seekErr != nil {
		return fmt.Errorf("rewind archive: %w", seekErr)
	}
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open model archive: %w", err)
	}
	defer gz.Close()
	// Bound decompression even though the archive has a pinned checksum.
	decoded := &io.LimitedReader{R: gz, N: 512 << 20}
	tr := tar.NewReader(decoded)
	found := make(map[string]bool)
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("extract model archive: %w", err)
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read model archive: %w", err)
		}
		clean := path.Clean(h.Name)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(h.Name, "\\\\:") {
			return fmt.Errorf("unsafe model archive entry %q", h.Name)
		}
		name := path.Base(clean)
		if name != modelFileName && name != tokenizerFileName {
			continue
		}
		// Chroma's archive stores these artifacts under onnx/. Flat archives are
		// also safe, but arbitrary nested paths must not match by basename alone.
		if clean != name && clean != "onnx/"+name {
			return fmt.Errorf("unexpected artifact path %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg || h.Size <= 0 || found[name] {
			return fmt.Errorf("invalid or duplicate model archive artifact %q", h.Name)
		}
		found[name] = true
		out, err := os.Create(filepath.Join(stage, name))
		if err != nil {
			return fmt.Errorf("stage artifact: %w", err)
		}
		_, copyErr := io.CopyN(out, tr, h.Size)
		closeErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("extract artifact: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close artifact: %w", closeErr)
		}
	}
	if decoded.N <= 0 {
		return fmt.Errorf("model archive exceeds decompressed size limit")
	}
	// Read through the gzip trailer with a bounded allowance for tar padding.
	if _, trailerErr := io.CopyN(io.Discard, gz, 1<<20); !errors.Is(trailerErr, io.EOF) {
		if trailerErr != nil {
			return fmt.Errorf("verify compressed archive: %w", trailerErr)
		}
		return fmt.Errorf("model archive has excessive trailing data")
	}
	if !found[modelFileName] || !found[tokenizerFileName] {
		return fmt.Errorf("model archive is incomplete: expected model.onnx and tokenizer.json")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("install model artifacts: %w", err)
	}
	for _, dest := range []string{paths.modelPath, paths.tokenizerPath} {
		if isReadableNonEmptyFile(dest) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("create artifact directory: %w", err)
		}
		if err := os.Rename(filepath.Join(stage, filepath.Base(dest)), dest); err != nil {
			return fmt.Errorf("install artifact: %w", err)
		}
	}
	return nil
}
