package fileparser

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/razvandimescu/gopdf/pdf"
	"github.com/xuri/excelize/v2"
)

func TestExtractPDFText(t *testing.T) {
	c := pdf.NewCreator()
	page := c.NewPage(595, 842)
	page.SetFont("Helvetica", 12)
	page.DrawText(72, 750, "hello from gopdf test")

	data, err := c.Build()
	if err != nil {
		t.Fatalf("build pdf: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	text, err := ExtractPDFText(path)
	if err != nil {
		t.Fatalf("ExtractPDFText: %v", err)
	}
	if !strings.Contains(text, "hello from gopdf test") {
		t.Fatalf("expected extracted text to contain sample text, got: %q", text)
	}
}

func TestExtractExcelText(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	if err := f.SetCellValue("Sheet1", "A1", "청약자격"); err != nil {
		t.Fatalf("set cell: %v", err)
	}
	if err := f.SetCellValue("Sheet1", "B1", "무주택세대주"); err != nil {
		t.Fatalf("set cell: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save excel: %v", err)
	}

	text, err := ExtractExcelText(path)
	if err != nil {
		t.Fatalf("ExtractExcelText: %v", err)
	}
	if !strings.Contains(text, "청약자격") || !strings.Contains(text, "무주택세대주") {
		t.Fatalf("expected extracted text to contain sample cells, got: %q", text)
	}
}

func TestStripNulBytes(t *testing.T) {
	// Some PDFs' font/encoding tables decode to NUL bytes, which
	// PostgreSQL's TEXT type rejects outright (SQLSTATE 22021) — this must
	// never reach the store layer.
	got := stripNulBytes("자격요건\x00: 무주택\x00세대주\x00")
	want := "자격요건: 무주택세대주"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractPDFImages(t *testing.T) {
	// Blank single-page PDF, then overlay a tiny in-memory PNG so there's an
	// embedded image object to extract.
	c := pdf.NewCreator()
	c.NewPage(200, 200)
	base, err := c.Build()
	if err != nil {
		t.Fatalf("build base pdf: %v", err)
	}

	img, err := pdf.LoadImageBytes(makeTestPNG(t))
	if err != nil {
		t.Fatalf("load image: %v", err)
	}

	ed := pdf.NewEditor(base)
	ed.AddImage(pdf.ImageOverlay{
		Page: 0, Image: img, CX: 100, CY: 100, Width: 50, Height: 50, Opacity: 1,
	})
	withImage, err := ed.Apply()
	if err != nil {
		t.Fatalf("apply overlay: %v", err)
	}

	pdfPath := filepath.Join(t.TempDir(), "with-image.pdf")
	if err := os.WriteFile(pdfPath, withImage, 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	imagesDir := filepath.Join(t.TempDir(), "images")
	names, err := ExtractPDFImages(pdfPath, imagesDir, "test123")
	if err != nil {
		t.Fatalf("ExtractPDFImages: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("got %d images, want 1: %v", len(names), names)
	}
	if !strings.HasPrefix(names[0], "test123-img") {
		t.Errorf("got name %q, want prefix test123-img", names[0])
	}
	if _, err := os.Stat(filepath.Join(imagesDir, names[0])); err != nil {
		t.Errorf("extracted image file missing: %v", err)
	}
}

func TestExtractPDFImages_NoImages(t *testing.T) {
	c := pdf.NewCreator()
	page := c.NewPage(200, 200)
	page.SetFont("Helvetica", 12)
	page.DrawText(10, 100, "no images here")
	data, err := c.Build()
	if err != nil {
		t.Fatalf("build pdf: %v", err)
	}

	pdfPath := filepath.Join(t.TempDir(), "no-image.pdf")
	if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	names, err := ExtractPDFImages(pdfPath, filepath.Join(t.TempDir(), "images"), "test456")
	if err != nil {
		t.Fatalf("ExtractPDFImages: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("got %d images, want 0: %v", len(names), names)
	}
}

func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}
