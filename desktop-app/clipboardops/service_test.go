package clipboardops

import "testing"

func TestSystemImageHash(t *testing.T) {
	s := New(nil)
	if got := s.SystemImageHash(nil); got != "" {
		t.Fatalf("expected empty hash, got %q", got)
	}

	data := make([]byte, 80)
	for i := range data {
		data[i] = byte(i)
	}

	if got := s.SystemImageHash(data); got == "" {
		t.Fatal("expected non-empty hash")
	}
}
