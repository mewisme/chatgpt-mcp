package plugin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestExtractArchiveRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("bad"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ExtractArchive(archive, "zip", filepath.Join(t.TempDir(), "extract")); err == nil {
		t.Fatal("archive traversal accepted")
	}
}

func TestSafePluginArchivePathRejectsAbsoluteDriveAndTraversal(t *testing.T) {
	for _, name := range []string{"/absolute", `C:\\escape`, "../escape", "nested/../../escape", "."} {
		if _, err := safePluginArchivePath(name); err == nil {
			t.Fatalf("unsafe archive path accepted: %q", name)
		}
	}
	if got, err := safePluginArchivePath("assets/app.js"); err != nil || got != "assets/app.js" {
		t.Fatalf("safe archive path = %q err=%v", got, err)
	}
}

func TestExtractArchiveRejectsSymlinkAndDuplicateEntries(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		write func(*zip.Writer) error
	}{{name: "symlink", write: func(writer *zip.Writer) error {
		header := &zip.FileHeader{Name: "link"}
		header.SetMode(os.ModeSymlink | 0777)
		entry, err := writer.CreateHeader(header)
		if err == nil {
			_, err = entry.Write([]byte("target"))
		}
		return err
	}}, {name: "special", write: func(writer *zip.Writer) error {
		header := &zip.FileHeader{Name: "pipe"}
		header.SetMode(os.ModeNamedPipe | 0600)
		_, err := writer.CreateHeader(header)
		return err
	}}, {name: "duplicate", write: func(writer *zip.Writer) error {
		for range 2 {
			entry, err := writer.Create("same.txt")
			if err != nil {
				return err
			}
			if _, err := entry.Write([]byte("x")); err != nil {
				return err
			}
		}
		return nil
	}}} {
		t.Run(fixture.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), fixture.name+".zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(file)
			if err := fixture.write(writer); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := ExtractArchive(archive, "zip", filepath.Join(t.TempDir(), "extract")); err == nil {
				t.Fatalf("%s archive accepted", fixture.name)
			}
		})
	}
}

func TestExtractArchiveRejectsTooManyEntries(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "many.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for i := 0; i <= maxPluginArchiveEntries; i++ {
		if _, err := writer.Create("files/" + strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ExtractArchive(archive, "zip", filepath.Join(t.TempDir(), "extract")); err == nil {
		t.Fatal("archive entry limit was not enforced")
	}
}

func TestAddExtractedBytesEnforcesLimits(t *testing.T) {
	if total, err := addExtractedBytes(10, 20, "ok"); err != nil || total != 30 {
		t.Fatalf("valid extracted bytes = %d err=%v", total, err)
	}
	if _, err := addExtractedBytes(0, -1, "negative"); err == nil {
		t.Fatal("negative archive entry size accepted")
	}
	if _, err := addExtractedBytes(0, maxPluginExtractedBytes+1, "large"); err == nil {
		t.Fatal("oversized archive entry accepted")
	}
	if _, err := addExtractedBytes(maxPluginExtractedBytes, 1, "total"); err == nil {
		t.Fatal("archive extracted total limit exceeded without error")
	}
}

func testZipBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
