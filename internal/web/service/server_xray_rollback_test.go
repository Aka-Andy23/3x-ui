package service

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractXrayCandidateAndAtomicBackup(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("xray")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("candidate")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	candidate, err := extractXrayCandidate(reader, "xray", dir)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "xray.previous")
	if err := copyFileAtomic(candidate, backup, 0o755, 1024); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "candidate" {
		t.Fatalf("backup=%q", body)
	}
	if err := copyFileAtomic(candidate, backup, 0o755, 2); err == nil {
		t.Fatal("oversized backup accepted")
	}
	body, err = os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "candidate" {
		t.Fatalf("failed copy replaced backup: %q", body)
	}
}
