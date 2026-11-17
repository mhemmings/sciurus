package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// maxDocumentBytes bounds both disk input and HTTP input while accommodating
// diagrams considerably larger than the bundled application schemas.
const maxDocumentBytes = 16 << 20

// errConflict marks an optimistic save rejected because its on-disk base changed.
var errConflict = errors.New("the model changed on disk; reopen it before saving to avoid overwriting those changes")

// revision identifies exact file bytes, including edits to whitespace or layout.
func revision(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// localPath resolves the parent once and rejects symlinks so subsequent saves
// replace the file explicitly selected by the user, never a symlink's target.
func localPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return abs, nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("model/output path must be a regular file, not a directory or symlink")
	}
	return abs, nil
}

// protectedOutputPath prevents headless conversions from replacing their own
// source files, including aliases through parent symlinks or hard links.
func protectedOutputPath(output string, inputs ...string) (string, error) {
	destination, err := localPath(output)
	if err != nil {
		return "", err
	}
	destinationInfo, err := os.Stat(destination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, input := range inputs {
		if input == "" {
			continue
		}
		source, err := localPath(input)
		if err != nil {
			return "", err
		}
		sourceInfo, err := os.Stat(source)
		if err != nil {
			return "", err
		}
		if source == destination || (destinationInfo != nil && os.SameFile(sourceInfo, destinationInfo)) {
			return "", errors.New("output must differ from every input file; refusing to overwrite the source")
		}
	}
	return destination, nil
}

// readFile accepts regular files only and avoids unbounded allocations from a
// malformed model or an accidentally selected device or huge SQL export.
func readFile(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("input must be a regular file, not a directory or symlink")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return nil, 0, err
	}
	if len(data) > maxDocumentBytes {
		return nil, 0, errors.New("file exceeds the 16 MiB input limit")
	}
	return data, info.Mode().Perm(), nil
}

// writeFile atomically replaces a file, preserving its permission bits. Passing
// expected enables optimistic concurrency; an empty revision means a new file.
// The temporary file is created alongside its target so rename stays atomic.
func writeFile(path string, data []byte, expected *string) (string, error) {
	current, mode, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		mode = 0600
		current = nil
	} else if err != nil {
		return "", err
	}
	base := ""
	if current != nil {
		base = revision(current)
	}
	if expected != nil && *expected != base {
		return "", errConflict
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".sciurus-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	err = temp.Chmod(mode)
	if err != nil {
		return "", err
	}
	_, err = temp.Write(data)
	if err != nil {
		return "", err
	}
	err = temp.Sync()
	if err != nil {
		return "", err
	}
	err = temp.Close()
	if err != nil {
		return "", err
	}
	// Check immediately before rename as well, detecting changes made while the
	// replacement was being prepared. Local editors should also use revisions.
	if expected != nil {
		latest, _, readErr := readFile(path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return "", readErr
		}
		latestRevision := ""
		if readErr == nil {
			latestRevision = revision(latest)
		}
		if latestRevision != *expected {
			return "", errConflict
		}
	}
	err = os.Rename(temp.Name(), path)
	if err != nil {
		return "", err
	}
	return revision(data), nil
}
