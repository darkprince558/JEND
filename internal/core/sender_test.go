package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressPath_Zip(t *testing.T) {
	// 1. Setup Test Dir
	srcDir := t.TempDir()
	file1 := filepath.Join(srcDir, "file1.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	subDir := filepath.Join(srcDir, "subdir")
	os.Mkdir(subDir, 0755)
	file2 := filepath.Join(subDir, "file2.txt")
	os.WriteFile(file2, []byte("content2"), 0644)

	// 2. Compress
	zipPath, err := CompressPath(srcDir, "zip")
	if err != nil {
		t.Fatalf("CompressPath failed: %v", err)
	}
	defer os.Remove(zipPath)

	// 3. Verify Zip
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("Failed to open zip: %v", err)
	}
	defer r.Close()

	foundCounts := 0
	for _, f := range r.File {
		// CompressPath includes the root folder, so we check suffixes
		if filepath.Base(f.Name) == "file1.txt" || filepath.Base(f.Name) == "file2.txt" {
			foundCounts++
		}
	}
	// Note: CompressPath creates archives where the root is the content of the dir, OR the dir itself?
	// Let's check implementation behavior expectation.
	// Usually it preserves relative paths.
	// If I compress "srcDir", does it put "srcDir/..." inside or just "..."?
	// Based on sender.go logic: `relPath, err := filepath.Rel(base, path)`
	// If base is srcDir, then files are at root.
	// So "file1.txt" and "subdir/file2.txt".

	if foundCounts < 2 {
		t.Errorf("Expected 2 specific files, found %d", foundCounts)
	}
}

func TestCompressPath_TarGz(t *testing.T) {
	// 1. Setup Test Dir
	srcDir := t.TempDir()
	file1 := filepath.Join(srcDir, "file1.txt")
	os.WriteFile(file1, []byte("content1"), 0644)

	// 2. Compress
	tarPath, err := CompressPath(srcDir, "tar.gz")
	if err != nil {
		t.Fatalf("CompressPath failed: %v", err)
	}
	defer os.Remove(tarPath)

	// 3. Verify TarGz
	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatalf("Failed to open tar.gz: %v", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Tar reading failed: %v", err)
		}
		if filepath.Base(header.Name) == "file1.txt" {
			found = true
		}
	}
	if !found {
		t.Error("Expected file1.txt in tarball")
	}
}
