package fileops

import (
	"archive/zip"
	"context"
	"desktop-app/api"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Service struct {
	client         *api.Client
	downloadFolder string
	mu             sync.RWMutex
}

func New(client *api.Client, downloadFolder string) *Service {
	return &Service{
		client:         client,
		downloadFolder: downloadFolder,
	}
}

func (s *Service) SetDownloadFolder(folder string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloadFolder = folder
}

func (s *Service) getDownloadFolder() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.downloadFolder
}

func (s *Service) ListFiles(ctx context.Context, path string) ([]api.FileInfo, error) {
	return s.client.ListFiles(ctx, path)
}

func (s *Service) Download(ctx context.Context, file api.FileInfo) error {
	return s.DownloadWithProgress(ctx, file, nil)
}

func (s *Service) DownloadWithProgress(ctx context.Context, file api.FileInfo, progressFn func(float64)) error {
	downloadFolder := s.getDownloadFolder()
	basePath := filepath.Join(downloadFolder, filepath.Base(file.Name))
	// Re-verify that it's within downloadFolder just in case
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(downloadFolder)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absBase, absRoot) {
		return fmt.Errorf("path traversal attempt blocked: %s", file.Name)
	}

	downloadPath, extractFolder := planDownloadTargets(basePath, file)

	if err := os.MkdirAll(filepath.Dir(downloadPath), 0o755); err != nil {
		return err
	}

	reader, err := s.client.DownloadFile(ctx, file.Name, "")
	if err != nil {
		return err
	}
	defer reader.Close()

	destFile, err := os.Create(downloadPath)
	if err != nil {
		return err
	}

	// Wrap reader with progress tracking if callback provided
	if progressFn != nil {
		reader = &progressReader{reader: reader, total: file.Size, progressFn: progressFn}
	}

	if _, err := io.Copy(destFile, reader); err != nil {
		_ = destFile.Close()
		return err
	}
	_ = destFile.Close()

	if file.IsDirectory {
		if err := unzip(downloadPath, extractFolder); err != nil {
			return err
		}
		_ = os.Remove(downloadPath)
	}
	return nil
}

// progressReader wraps an io.Reader to report progress
type progressReader struct {
	reader     io.ReadCloser
	total      int64
	read       int64
	progressFn func(float64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.read += int64(n)
		if pr.total > 0 {
			pr.progressFn(float64(pr.read) / float64(pr.total))
		}
	}
	return n, err
}

func (pr *progressReader) Close() error {
	return pr.reader.Close()
}

func (s *Service) UploadFile(ctx context.Context, filename string, reader io.Reader) error {
	return s.client.UploadFile(ctx, filename, reader)
}

func (s *Service) UploadFolder(ctx context.Context, folderPath string) error {
	return s.UploadFolderWithPrefix(ctx, folderPath, "")
}

func (s *Service) UploadFolderWithPrefix(ctx context.Context, folderPath string, prefix string) error {
	folderName := filepath.Base(folderPath)
	return filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Check context at each file start
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath, _ := filepath.Rel(folderPath, path)
		fullRelPath := prefix + filepath.Join(folderName, relPath)
		// Ensure forward slashes for server compatibility
		fullRelPath = filepath.ToSlash(fullRelPath)

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		if err := s.client.UploadFile(ctx, fullRelPath, file); err != nil {
			log.Printf("Failed to upload %s: %v", relPath, err)
			return err
		}
		return nil
	})
}

func (s *Service) DeleteFile(ctx context.Context, filename string) error {
	return s.client.DeleteFile(ctx, filename)
}

func planDownloadTargets(basePath string, file api.FileInfo) (string, string) {
	if file.IsDirectory {
		targetFolder := uniqueFilename(basePath)
		return targetFolder + ".zip", targetFolder
	}
	return uniqueFilename(basePath), ""
}

func uniqueFilename(basePath string) string {
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return basePath
	}

	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	nameWithoutExt := filepath.Base(basePath)
	nameWithoutExt = nameWithoutExt[:len(nameWithoutExt)-len(ext)]

	counter := 1
	for {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", nameWithoutExt, counter, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		counter++
	}
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	// P1: ZIP extraction guardrails
	const (
		MaxFiles          = 10000
		MaxTotalSize      = 1024 * 1024 * 1024 // 1GB
		MaxPerEntrySize   = 200 * 1024 * 1024  // 200MB
	)

	if len(r.File) > MaxFiles {
		return fmt.Errorf("zip contains too many files (%d > %d)", len(r.File), MaxFiles)
	}

	var totalUncompressedSize int64

	extractAndWriteFile := func(f *zip.File) error {
		// Basic per-entry size check (header-based)
		if f.UncompressedSize64 > MaxPerEntrySize {
			return fmt.Errorf("file %s is too large (%d > %d)", f.Name, f.UncompressedSize64, MaxPerEntrySize)
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			return os.MkdirAll(path, f.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(path), f.Mode()); err != nil {
			return err
		}

		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		defer out.Close()

		// P1: Use LimitReader to enforce per-entry cap and update cumulative total
		lr := io.LimitReader(rc, MaxPerEntrySize)
		n, err := io.Copy(out, lr)
		if err != nil {
			return err
		}
		
		// If we hit the limit, it means the file was larger than MaxPerEntrySize
		if n >= MaxPerEntrySize {
			// Check if there is still data in the original reader
			tmp := make([]byte, 1)
			if tn, _ := rc.Read(tmp); tn > 0 {
				return fmt.Errorf("file %s exceeded maximum entry size %d", f.Name, MaxPerEntrySize)
			}
		}

		totalUncompressedSize += n
		if totalUncompressedSize > MaxTotalSize {
			return fmt.Errorf("cumulative uncompressed size exceeded limit (%d > %d)", totalUncompressedSize, MaxTotalSize)
		}

		return nil
	}

	for _, f := range r.File {
		if err := extractAndWriteFile(f); err != nil {
			return err
		}
	}
	return nil
}

func DownloadPathForTest(basePath string, file api.FileInfo) (string, string) {
	return planDownloadTargets(basePath, file)
}
