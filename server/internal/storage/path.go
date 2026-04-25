package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var uniqueMu sync.Mutex

func UniqueFilename(path string) string {
	uniqueMu.Lock()
	defer uniqueMu.Unlock()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	nameWithoutExt := filepath.Base(path)
	if len(ext) > 0 {
		nameWithoutExt = nameWithoutExt[:len(nameWithoutExt)-len(ext)]
	}

	for counter := 1; ; counter++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", nameWithoutExt, counter, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// CreateAtomic handles race-free file creation by combining uniqueness check and file creation
func CreateAtomic(path string) (*os.File, string, error) {
	uniqueMu.Lock()
	defer uniqueMu.Unlock()

	// P2: Explicitly check for directory collision
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		// It's a directory, trigger unique naming loop below
	} else {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			return f, path, nil
		}
		if !os.IsExist(err) && !strings.Contains(err.Error(), "is a directory") {
			return nil, "", err
		}
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	nameWithoutExt := filepath.Base(path)
	if len(ext) > 0 {
		nameWithoutExt = nameWithoutExt[:len(nameWithoutExt)-len(ext)]
	}

	for counter := 1; ; counter++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", nameWithoutExt, counter, ext))
		
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			continue // Skip directories
		}

		f, err := os.OpenFile(candidate, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			return f, candidate, nil
		}
		if !os.IsExist(err) && !strings.Contains(err.Error(), "is a directory") {
			return nil, "", err
		}
	}
}

func ResolveWithinRoot(rootDir, relPath string) (string, error) {
	cleanRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(cleanRoot, filepath.Clean(relPath))
	
	// Check for symlinks and resolve them to verify they stay within root
	if info, err := os.Lstat(fullPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(fullPath)
			if err != nil {
				return "", err
			}
			fullPath = resolved
		}
	}

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(cleanRoot, absPath)
	if err != nil {
		return "", err
	}
	
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// On Windows, filepath.Rel might return an absolute path if they are on different drives
		// or if case doesn't match perfectly. But filepath.Abs already handled that.
		// However, let's be extra safe.
		return "", fmt.Errorf("path escapes root")
	}
	
	return absPath, nil
}
