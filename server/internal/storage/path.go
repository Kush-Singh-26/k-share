package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func UniqueFilename(path string) string {
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

func ResolveWithinRoot(rootDir, relPath string) (string, error) {
	cleanRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(cleanRoot, filepath.Clean(relPath))
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	sepRoot := cleanRoot + string(filepath.Separator)
	if !strings.HasPrefix(absPath, sepRoot) && absPath != cleanRoot {
		return "", fmt.Errorf("path escapes root")
	}
	return absPath, nil
}
