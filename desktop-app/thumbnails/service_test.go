package thumbnails

import "testing"

func TestHashFilenameStable(t *testing.T) {
	got1 := HashFilenameForTest("Public/photo.png")
	got2 := HashFilenameForTest("Public/photo.png")
	if got1 == "" || got1 != got2 {
		t.Fatalf("expected stable non-empty hash, got %q and %q", got1, got2)
	}
}

func TestIsImageFile(t *testing.T) {
	svc := New(nil)
	if !svc.IsImageFile("image.PNG") {
		t.Fatal("expected png to be detected as image")
	}
	if svc.IsImageFile("doc.txt") {
		t.Fatal("expected txt not to be detected as image")
	}
}
