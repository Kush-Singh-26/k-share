package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Item struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type Store struct {
	appDir string
	mu     sync.RWMutex
	items  []Item
	loaded bool
}

func New(appDir string) *Store {
	return &Store{appDir: appDir}
}

func (s *Store) Path() string {
	return filepath.Join(s.appDir, "clipboard_history.json")
}

func (s *Store) load() error {
	if s.loaded {
		return nil
	}
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			s.items = []Item{}
			s.loaded = true
			return nil
		}
		return err
	}

	if err := json.Unmarshal(data, &s.items); err != nil {
		return err
	}
	s.loaded = true
	return nil
}

func (s *Store) save() error {
	data, err := json.Marshal(s.items)
	if err != nil {
		return err
	}
	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path())
}

func (s *Store) List() ([]Item, error) {
	// P1: Handle lazy loading safely without mutation in RLock
	s.mu.RLock()
	if !s.loaded {
		s.mu.RUnlock()
		s.mu.Lock()
		// Double check after acquiring full lock
		if err := s.load(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.mu.Unlock()
		s.mu.RLock()
	}
	defer s.mu.RUnlock()

	// Return a copy to avoid race on slice contents if caller modifies it
	res := make([]Item, len(s.items))
	copy(res, s.items)
	return res, nil
}

func (s *Store) Add(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.load(); err != nil {
		return err
	}

	if len(s.items) > 0 && s.items[0].Text == text {
		return nil
	}

	ts := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s%d", text, ts)))
	
	item := Item{
		ID:        hex.EncodeToString(hash[:])[:12],
		Text:      text,
		Timestamp: time.Now(),
	}

	s.items = append([]Item{item}, s.items...)
	if len(s.items) > 20 {
		s.items = s.items[:20]
	}

	return s.save()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.load(); err != nil {
		return err
	}

	filtered := make([]Item, 0, len(s.items))
	found := false
	for _, item := range s.items {
		if item.ID != id {
			filtered = append(filtered, item)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("item not found")
	}

	s.items = filtered
	return s.save()
}
