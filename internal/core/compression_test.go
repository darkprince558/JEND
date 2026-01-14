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

func TestCompressPath(t *testing.T) {
	// Setup Test Directory
	testDir := t.TempDir()

	// Create some files
	err := os.Mkdir(filepath.Join(testDir, "subdir"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(testDir, "subdir", "file2.txt"), []byte("world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test Tar.gz Compression
	tarPath, err := CompressPath(testDir, "tar.gz")
	if err != nil {
		t.Fatalf("CompressPath(tar.gz) failed: %v", err)
	}
	defer os.Remove(tarPath)

	// Verify Tar integrity
	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)

	foundFile1 := false
	foundFile2 := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		// Expect names like "subdir/file2.txt" inside because CompressPath uses relative path
		// Actually CompressPath includes the base directory name if it's a directory
		// Let's check what we expect. Logic says `filepath.Rel(filepath.Dir(filePath), path)`
		// If filePath is /tmp/testDir, Dir is /tmp, Rel is testDir/file1.txt
		// So we verify we see "testDir/..." prefix

		t.Logf("Tar Entry: %s", header.Name)
		if filepath.Base(header.Name) == "file1.txt" {
			foundFile1 = true
		}
		if filepath.Base(header.Name) == "file2.txt" {
			foundFile2 = true
		}
	}

	if !foundFile1 || !foundFile2 {
		t.Errorf("Tar missing files. Found1: %v, Found2: %v", foundFile1, foundFile2)
	}

	// Test Zip Compression
	zipPath, err := CompressPath(testDir, "zip")
	if err != nil {
		t.Fatalf("CompressPath(zip) failed: %v", err)
	}
	defer os.Remove(zipPath)

	// Verify Zip Integrity
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	foundFile1 = false
	foundFile2 = false

	for _, f := range zr.File {
		t.Logf("Zip Entry: %s", f.Name)
		if filepath.Base(f.Name) == "file1.txt" {
			foundFile1 = true
		}
		if filepath.Base(f.Name) == "file2.txt" {
			foundFile2 = true
		}
	}

	if !foundFile1 || !foundFile2 {
		t.Errorf("Zip missing files. Found1: %v, Found2: %v", foundFile1, foundFile2)
	}
}
