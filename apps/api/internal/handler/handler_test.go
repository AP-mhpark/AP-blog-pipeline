package handler

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeTestImage(t *testing.T, dir, name string, size int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o644); err != nil {
		t.Fatalf("write test image %s: %v", name, err)
	}
}

func TestLoadDraftImages_FiltersBelowSizeThreshold(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	writeTestImage(t, imagesDir, "icon.png", minDraftImageBytes-1)
	writeTestImage(t, imagesDir, "photo.png", minDraftImageBytes+1)

	got := loadDraftImages(dir, []string{"icon.png", "photo.png"})
	if len(got) != 1 {
		t.Fatalf("got %d images, want 1 (only photo.png clears the threshold): %+v", len(got), got)
	}
	if got[0].Filename != "photo.png" {
		t.Fatalf("got filename %q, want photo.png", got[0].Filename)
	}
	if got[0].MediaType != "image/png" {
		t.Fatalf("got media type %q, want image/png", got[0].MediaType)
	}
}

func TestLoadDraftImages_MediaTypeByExtension(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	content := make([]byte, minDraftImageBytes+100)
	for i := range content {
		content[i] = byte(i)
	}
	writeFile := func(name string) {
		if err := os.WriteFile(filepath.Join(imagesDir, name), content, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile("a.png")
	writeFile("b.jpg")
	writeFile("c.jpeg")
	writeFile("d.gif") // unsupported, must be skipped

	got := loadDraftImages(dir, []string{"a.png", "b.jpg", "c.jpeg", "d.gif"})
	if len(got) != 3 {
		t.Fatalf("got %d images, want 3 (gif unsupported): %+v", len(got), got)
	}

	want := map[string]string{"a.png": "image/png", "b.jpg": "image/jpeg", "c.jpeg": "image/jpeg"}
	for _, img := range got {
		if img.MediaType != want[img.Filename] {
			t.Errorf("filename %s: got media type %q, want %q", img.Filename, img.MediaType, want[img.Filename])
		}
		decoded, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			t.Fatalf("decode base64 for %s: %v", img.Filename, err)
		}
		if string(decoded) != string(content) {
			t.Errorf("filename %s: base64 round-trip mismatch", img.Filename)
		}
	}
}

func TestLoadDraftImages_SkipsMissingFileNonFatally(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	writeTestImage(t, imagesDir, "exists.png", minDraftImageBytes+1)

	got := loadDraftImages(dir, []string{"missing.png", "exists.png"})
	if len(got) != 1 {
		t.Fatalf("got %d images, want 1 (missing.png skipped): %+v", len(got), got)
	}
	if got[0].Filename != "exists.png" {
		t.Fatalf("got filename %q, want exists.png", got[0].Filename)
	}
}

func TestLoadDraftImages_CapsAtMaxDraftImages(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}

	var names []string
	for i := 0; i < maxDraftImages+5; i++ {
		name := fmt.Sprintf("img%d.png", i)
		writeTestImage(t, imagesDir, name, minDraftImageBytes+1)
		names = append(names, name)
	}

	got := loadDraftImages(dir, names)
	if len(got) != maxDraftImages {
		t.Fatalf("got %d images, want cap of %d", len(got), maxDraftImages)
	}
}
