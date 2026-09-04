package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownloaderDownload(t *testing.T) {
	assetName, err := CurrentAssetName("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	archive := releaseArchive(t, assetName, []byte("release-binary"))
	archiveHash := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(archiveHash[:]) + "  " + assetName + "\n")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + assetName:
			_, _ = w.Write(archive)
		case "/checksums.txt":
			_, _ = w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	parent := t.TempDir()
	release := Release{Version: "v1.2.3", ArchiveName: assetName, ArchiveURL: server.URL + "/" + assetName, ChecksumName: "checksums.txt", ChecksumURL: server.URL + "/checksums.txt"}
	artifact, err := (Downloader{HTTPClient: server.Client(), TempDir: parent}).Download(context.Background(), release)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(artifact.Binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "release-binary" {
		t.Fatalf("binary = %q", content)
	}
	if !strings.HasPrefix(artifact.Dir, parent) {
		t.Fatalf("artifact dir = %q", artifact.Dir)
	}
	if err := artifact.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(artifact.Dir); !os.IsNotExist(err) {
		t.Fatalf("artifact directory remains: %v", err)
	}
}

func TestDownloaderCleansUpOnChecksumMismatch(t *testing.T) {
	assetName, err := CurrentAssetName("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	archive := releaseArchive(t, assetName, []byte("release-binary"))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			fmt.Fprintf(w, "%064d  %s\n", 0, assetName)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	parent := t.TempDir()
	release := Release{Version: "v1.2.3", ArchiveName: assetName, ArchiveURL: server.URL + "/" + assetName, ChecksumName: "checksums.txt", ChecksumURL: server.URL + "/checksums.txt"}
	if _, err := (Downloader{HTTPClient: server.Client(), TempDir: parent}).Download(context.Background(), release); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed download left temporary files: %v", entries)
	}
}

func TestDownloaderRejectsHTTP(t *testing.T) {
	assetName, err := CurrentAssetName("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	release := Release{Version: "v1.2.3", ArchiveName: assetName, ArchiveURL: "http://example.test/" + assetName, ChecksumName: "checksums.txt", ChecksumURL: "https://example.test/checksums.txt"}
	if _, err := (Downloader{TempDir: t.TempDir()}).Download(context.Background(), release); err == nil {
		t.Fatal("HTTP release URL was accepted")
	}
}

func TestDownloaderRejectsUnexpectedAssetName(t *testing.T) {
	release := Release{Version: "v1.2.3", ArchiveName: "unexpected.tar.gz", ArchiveURL: "https://example.test/unexpected.tar.gz", ChecksumName: "checksums.txt", ChecksumURL: "https://example.test/checksums.txt"}
	if _, err := (Downloader{TempDir: t.TempDir()}).Download(context.Background(), release); err == nil {
		t.Fatal("unexpected release asset name was accepted")
	}
}

func TestDownloaderRejectsHTTPRedirect(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("bad")) }))
	defer httpServer.Close()
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, httpServer.URL, http.StatusFound) }))
	defer tlsServer.Close()
	destination := filepath.Join(t.TempDir(), "download")
	if err := (Downloader{HTTPClient: tlsServer.Client()}).downloadFile(context.Background(), tlsServer.URL, destination, 1024); err == nil {
		t.Fatal("HTTP redirect was accepted")
	}
}

func TestDownloaderEnforcesBodyLimitWithoutContentLength(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "")
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "download")
	if err := (Downloader{HTTPClient: server.Client()}).downloadFile(context.Background(), server.URL, destination, 4); err == nil {
		t.Fatal("oversized body was accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial download remains: %v", err)
	}
}

func releaseArchive(t *testing.T, assetName string, binary []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, assetName)
	if runtime.GOOS == "windows" {
		writeZipArchive(t, path, []zipEntry{{name: "chatgpt-mcp.exe", content: binary}})
	} else {
		writeTarArchive(t, path, []tarEntry{{name: "chatgpt-mcp", content: binary}})
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
