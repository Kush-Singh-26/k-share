package files

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	serverstorage "github.com/Kush-Singh-26/k-share/server/internal/storage"
)

type Entry struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
	Size        int64  `json:"size"`
	ModTime     string `json:"modTime"`
}

func List(rootDir string, subPath string) ([]Entry, error) {
	var entries []Entry

	targetPath := rootDir
	prefix := ""

	if subPath != "" {
		resolved, err := serverstorage.ResolveWithinRoot(rootDir, subPath)
		if err != nil {
			return nil, err
		}
		targetPath = resolved
		prefix = filepath.ToSlash(filepath.Clean(subPath))
		if prefix == "." {
			prefix = ""
		} else if prefix != "" {
			prefix += "/"
		}
	}

	dirEntries, err := os.ReadDir(targetPath)
	if err != nil {
		if os.IsNotExist(err) && targetPath != rootDir {
			return entries, nil
		}
		return nil, err
	}

	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := info.Name()

		if name == ".trash" {
			continue
		}

		entries = append(entries, Entry{
			Name:        prefix + name,
			IsDirectory: info.IsDir(),
			Size:        info.Size(),
			ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return entries, nil
}

func sanitizeFilename(path string) string {
	// 1. Clean the path and convert to logical forward slashes
	path = filepath.ToSlash(filepath.Clean(path))
	
	// 2. Prevent escaping root via .. or absolute paths
	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		// If it's malicious, just fallback to base name for safety
		path = filepath.Base(path)
	}

	// 3. Sanitize each segment
	segments := strings.Split(path, "/")
	var sanitized []string
	for _, s := range segments {
		if s == "" || s == "." || s == ".." {
			continue
		}
		var b strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
				r == '.' || r == '-' || r == '_' || r == ' ' || r == '(' || r == ')' {
				b.WriteRune(r)
			}
		}
		clean := strings.TrimSpace(b.String())
		if clean != "" {
			sanitized = append(sanitized, clean)
		}
	}
	if len(sanitized) == 0 {
		return "unnamed"
	}
	result := strings.Join(sanitized, "/")
	// 4. Final guard: ensure no trailing/leading dots that could be exploited on Windows
	result = strings.Trim(result, ".")
	if result == "" {
		return "unnamed"
	}
	return result
}

func Upload(rootDir, filename string, body io.Reader) error {
	filename = sanitizeFilename(filename)
	if filename == "" || filename == "unnamed" {
		filename = "upload_" + time.Now().Format("20060102_150405")
	}

	destPath, err := serverstorage.ResolveWithinRoot(rootDir, filename)
	if err != nil {
		return fmt.Errorf("invalid filename: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	destFile, finalPath, err := serverstorage.CreateAtomic(destPath)
	if err != nil {
		return err
	}
	destPath = finalPath
	
	success := false
	defer func() {
		destFile.Close()
		if !success {
			_ = os.Remove(destPath)
		}
	}()

	if _, err = io.Copy(destFile, body); err != nil {
		return err
	}
	success = true
	return nil
}

func Delete(rootDir, filename string) error {
	filename = sanitizeFilename(filename)
	fullPath, err := serverstorage.ResolveWithinRoot(rootDir, filename)
	if err != nil {
		return err
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// File already gone or doesn't exist - return success for idempotency
		return nil
	}

	trashDir := filepath.Join(rootDir, ".trash")
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return err
	}

	baseName := filepath.Base(filename)
	trashPath := filepath.Join(trashDir, baseName)
	trashPath = serverstorage.UniqueFilename(trashPath)

	// Try rename first; if cross-device, fall back to copy+delete
	if err := os.Rename(fullPath, trashPath); err != nil {
		if err := copyFile(fullPath, trashPath); err != nil {
			return fmt.Errorf("failed to move to trash: %w", err)
		}
		_ = os.Remove(fullPath)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// Walk walks all files under rootDir and calls fn for each entry.
func Walk(rootDir string, fn func(Entry) error) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == rootDir {
			return nil
		}
		name := info.Name()
		if name == ".trash" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(rootDir, path)
		return fn(Entry{
			Name:        filepath.ToSlash(rel),
			IsDirectory: info.IsDir(),
			Size:        info.Size(),
			ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
		})
	})
}

func ServeDownload(rootDir, relPath string, w http.ResponseWriter, r *http.Request) error {
	fullPath, err := serverstorage.ResolveWithinRoot(rootDir, relPath)
	if err != nil {
		http.Error(w, "Access denied", http.StatusForbidden)
		return nil
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return nil
	}

	if info.IsDir() {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", filepath.Base(fullPath)))
		zw := zip.NewWriter(w)
		defer zw.Close()

		return filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(fullPath, path)
			if rel == "." {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if info.Mode()&os.ModeSymlink != 0 {
				return nil // Skip symlinks in zip to prevent leakage/loops
			}
			if info.IsDir() {
				_, err = zw.Create(rel + "/")
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			
			dest, err := zw.Create(rel)
			if err != nil {
				f.Close()
				return err
			}
			_, err = io.Copy(dest, f)
			f.Close()
			return err
		})
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(fullPath)))
	http.ServeFile(w, r, fullPath)
	return nil
}
