package clipboardstore

import (
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	appDir string
	mu     sync.Mutex
}

func New(appDir string) *Store {
	return &Store{appDir: appDir}
}

func (s *Store) TextPath(name string) string {
	return filepath.Join(s.appDir, name)
}

func (s *Store) ReadText(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.TextPath(name))
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return data, err
}

func (s *Store) WriteText(name string, data []byte, appendMode bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetPath := s.TextPath(name)
	finalData := data
	if appendMode {
		currentData, err := os.ReadFile(targetPath)
		if err == nil && len(currentData) > 0 {
			finalData = append(currentData, []byte("\n")...)
			finalData = append(finalData, data...)
		}
	}
	return atomicWriteFile(targetPath, finalData, 0o644)
}

func (s *Store) ImagePath() string {
	return filepath.Join(s.appDir, "clipboard_image.png")
}

func (s *Store) ReadImage() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.ImagePath())
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return data, err
}

func (s *Store) WriteImage(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return atomicWriteFile(s.ImagePath(), data, 0o644)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
