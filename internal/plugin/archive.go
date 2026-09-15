package plugin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxPluginExtractedBytes int64 = 512 << 20
	maxPluginArchiveEntries       = 10000
)

func ExtractArchive(archivePath, archiveType, destination string) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	switch archiveType {
	case "zip":
		return extractPluginZip(archivePath, destination)
	case "tar.gz":
		return extractPluginTar(archivePath, destination)
	default:
		return fmt.Errorf("unsupported plugin archive type: %q", archiveType)
	}
}

func extractPluginZip(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) > maxPluginArchiveEntries {
		return errors.New("plugin archive has too many entries")
	}
	seen := map[string]bool{}
	var total int64
	for _, entry := range reader.File {
		name, err := safePluginArchivePath(entry.Name)
		if err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("plugin archive contains duplicate entry: %s", name)
		}
		seen[name] = true
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || !mode.IsRegular() && !mode.IsDir() {
			return fmt.Errorf("plugin archive contains unsupported entry type: %s", name)
		}
		size := entry.FileInfo().Size()
		total, err = addExtractedBytes(total, size, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		stream, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeArchiveFile(target, stream, size, mode.Perm())
		closeErr := stream.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractPluginTar(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	seen := map[string]bool{}
	var total int64
	entries := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		entries++
		if entries > maxPluginArchiveEntries {
			return errors.New("plugin archive has too many entries")
		}
		name, err := safePluginArchivePath(header.Name)
		if err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("plugin archive contains duplicate entry: %s", name)
		}
		seen[name] = true
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filepath.Join(destination, filepath.FromSlash(name)), 0700); err != nil {
				return err
			}
			continue
		case tar.TypeReg, 0:
		default:
			return fmt.Errorf("plugin archive contains unsupported entry type: %s", name)
		}
		total, err = addExtractedBytes(total, header.Size, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := writeArchiveFile(target, reader, header.Size, header.FileInfo().Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func addExtractedBytes(total, size int64, name string) (int64, error) {
	if size < 0 || size > maxPluginExtractedBytes {
		return 0, fmt.Errorf("plugin archive entry has invalid size: %s", name)
	}
	if total > maxPluginExtractedBytes-size {
		return 0, errors.New("plugin archive exceeds extracted size limit")
	}
	return total + size, nil
}

func safePluginArchivePath(name string) (string, error) {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe plugin archive path: %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe plugin archive path: %q", name)
		}
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe plugin archive path: %q", name)
	}
	return clean, nil
}

func writeArchiveFile(path string, reader io.Reader, size int64, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode&0777)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(reader, size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != size {
		return fmt.Errorf("plugin archive entry size mismatch for %s", path)
	}
	return nil
}
