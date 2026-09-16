package licenseinventory

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	EnvInventory    = "CGM_LICENSE_INVENTORY"
	EnvInventoryOff = "off"
)

type ExtraFile struct {
	Name string
	Path string
}

func InventoryDisabled() bool {
	return os.Getenv(EnvInventory) == EnvInventoryOff
}

func ExtraFiles(root, artifactID string) ([]ExtraFile, error) {
	if InventoryDisabled() {
		return nil, nil
	}
	dir := filepath.Join(root, "dist", "licenses", artifactID)
	if _, err := os.Stat(filepath.Join(dir, "NOTICE")); err != nil {
		cmd := exec.Command("go", "run", "./internal/licenseinventory/cmd/license-inventory", "--artifact", artifactID, "--out", dir)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("generate %s license inventory: %w\n%s", artifactID, err, out)
		}
	}
	files := []ExtraFile{{Name: "LICENSE", Path: filepath.Join(root, "LICENSE")}}
	for _, name := range []string{"NOTICE", "licenses.txt", "sbom.spdx.json"} {
		files = append(files, ExtraFile{Name: name, Path: filepath.Join(dir, name)})
	}
	for _, file := range files {
		if _, err := os.Stat(file.Path); err != nil {
			return nil, fmt.Errorf("compliance file %s: %w", file.Name, err)
		}
	}
	return files, nil
}

func AddZipFiles(writer *zip.Writer, files []ExtraFile, modified time.Time) error {
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if err != nil {
			return err
		}
		header := &zip.FileHeader{Name: file.Name, Method: zip.Deflate, Modified: modified}
		header.SetMode(info.Mode())
		destination, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(file.Path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
