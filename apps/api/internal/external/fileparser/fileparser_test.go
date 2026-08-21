package fileparser

import (
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
