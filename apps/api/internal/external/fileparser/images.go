package fileparser

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// ExtractPDFImages extracts images embedded in the PDF at pdfPath into
// imagesDir (created if needed, flat and shared across uploads), renaming
// each to "<namePrefix>-imgN.<ext>" so results stay unique across uploads
// and traceable to their source PDF. Returns the bare filenames (not full
// paths) written into imagesDir — the same names both the drafting prompt
// and the frontend use to reference them.
//
// Many PDFs legitimately yield zero images: tables/figures in government
// notices are usually drawn as native PDF text+lines rather than embedded
// raster images, so this only catches things like photos or maps that were
// actually inserted as image objects.
func ExtractPDFImages(pdfPath, imagesDir, namePrefix string) ([]string, error) {
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create images dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "pdf-images-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := api.ExtractImagesFile(pdfPath, tmpDir, nil, nil); err != nil {
		return nil, fmt.Errorf("extract images: %w", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("read extracted images: %w", err)
	}

	names := make([]string, 0, len(entries))
	i := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		i++
		newName := fmt.Sprintf("%s-img%d%s", namePrefix, i, filepath.Ext(entry.Name()))
		if err := copyFile(filepath.Join(tmpDir, entry.Name()), filepath.Join(imagesDir, newName)); err != nil {
			return nil, fmt.Errorf("save extracted image: %w", err)
		}
		names = append(names, newName)
	}
	return names, nil
}

// copyFile copies src to dst. Used instead of os.Rename because src (in a
// temp dir) and dst (under the configured upload dir) may be on different
// filesystems, where Rename fails with "invalid cross-device link".
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

	_, err = io.Copy(out, in)
	return err
}
