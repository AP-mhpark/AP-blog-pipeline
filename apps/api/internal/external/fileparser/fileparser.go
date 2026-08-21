package fileparser

import (
	"fmt"
	"strings"

	"github.com/razvandimescu/gopdf/pdf"
	"github.com/xuri/excelize/v2"
)

// ExtractPDFText extracts the full text content of a PDF file at path.
func ExtractPDFText(path string) (string, error) {
	doc, err := pdf.OpenFile(path)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	text, err := doc.Text()
	if err != nil {
		return "", fmt.Errorf("extract pdf text: %w", err)
	}
	return text, nil
}

// ExtractExcelText reads every sheet of the Excel file at path and serializes
// its cell values as tab-separated rows, one sheet block per sheet.
func ExtractExcelText(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", fmt.Errorf("open excel: %w", err)
	}
	defer func() { _ = f.Close() }()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return "", fmt.Errorf("read sheet %q: %w", sheet, err)
		}
		sb.WriteString(sheet + "\n")
		for _, row := range rows {
			sb.WriteString(strings.Join(row, "\t"))
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}
