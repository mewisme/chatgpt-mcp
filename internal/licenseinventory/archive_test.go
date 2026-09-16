package licenseinventory

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtraFilesDisabled(t *testing.T) {
	t.Setenv(EnvInventory, EnvInventoryOff)
	files, err := ExtraFiles(t.TempDir(), "core")
	if err != nil || files != nil {
		t.Fatalf("disabled extras = %#v, %v", files, err)
	}
}

func TestAddZipFilesUsesArchiveNames(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "NOTICE")
	if err := os.WriteFile(path, []byte("notice"), 0600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	if err := AddZipFiles(writer, []ExtraFile{{Name: "NOTICE", Path: path}}, time.Unix(0, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 1 || reader.File[0].Name != "NOTICE" {
		t.Fatalf("zip = %#v", reader.File)
	}
}
