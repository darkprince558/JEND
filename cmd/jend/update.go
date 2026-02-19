package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/darkprince558/jend/internal/ui"
	"github.com/spf13/cobra"
)

const (
	githubRepo   = "darkprince558/JEND"
	githubAPIURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

// GitHub API response (only fields we need)
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update JEND to the latest version",
	Long:  `Check GitHub for the latest release and update the JEND binary in-place.`,
	Run:   runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) {
	bold := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorText)
	accent := lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ui.ColorSubtext)
	success := lipgloss.NewStyle().Foreground(ui.ColorSuccess).Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(ui.ColorError).Bold(true)

	fmt.Println(bold.Render("Checking for updates..."))

	// 1. Fetch latest release info
	release, err := fetchLatestRelease()
	if err != nil {
		fmt.Println(errStyle.Render("Failed to check for updates: " + err.Error()))
		os.Exit(1)
	}

	latestTag := release.TagName
	latestVer := strings.TrimPrefix(latestTag, "v")
	currentVer := strings.TrimPrefix(version, "v")

	fmt.Println(dim.Render("  Current: ") + accent.Render("v"+currentVer))
	fmt.Println(dim.Render("  Latest:  ") + accent.Render("v"+latestVer))

	if currentVer == latestVer && version != "dev" {
		fmt.Println(success.Render("\nAlready up to date."))
		return
	}

	// 2. Find the right asset for this OS/arch
	assetName := getAssetName()
	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		fmt.Println(errStyle.Render("No release found for " + runtime.GOOS + "/" + runtime.GOARCH))
		fmt.Println(dim.Render("Expected asset: " + assetName))
		os.Exit(1)
	}

	fmt.Println(bold.Render("\nDownloading " + assetName + "..."))

	// 3. Download to temp file
	tmpFile, err := downloadAsset(downloadURL)
	if err != nil {
		fmt.Println(errStyle.Render("Download failed: " + err.Error()))
		os.Exit(1)
	}
	defer os.Remove(tmpFile)

	// 4. Extract the binary
	binaryPath, err := extractBinary(tmpFile, assetName)
	if err != nil {
		fmt.Println(errStyle.Render("Extract failed: " + err.Error()))
		os.Exit(1)
	}
	defer os.Remove(binaryPath)

	// 5. Replace current binary
	currentBinary, err := os.Executable()
	if err != nil {
		fmt.Println(errStyle.Render("Cannot locate current binary: " + err.Error()))
		os.Exit(1)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		fmt.Println(errStyle.Render("Cannot resolve binary path: " + err.Error()))
		os.Exit(1)
	}

	err = replaceBinary(currentBinary, binaryPath)
	if err != nil {
		fmt.Println(errStyle.Render("Update failed: " + err.Error()))
		fmt.Println(dim.Render("Try running with sudo: sudo jend update"))
		os.Exit(1)
	}

	fmt.Println(success.Render("\nUpdated to v" + latestVer))
}

func fetchLatestRelease() (*ghRelease, error) {
	resp, err := http.Get(githubAPIURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return &release, nil
}

func getAssetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go's names to release asset names
	osName := ""
	switch goos {
	case "darwin":
		osName = "Darwin"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	default:
		osName = goos
	}

	archName := ""
	switch goarch {
	case "amd64":
		archName = "x86_64"
	case "arm64":
		archName = "arm64"
	default:
		archName = goarch
	}

	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("jend_%s_%s.%s", osName, archName, ext)
}

func downloadAsset(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "jend-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func extractBinary(archivePath, assetName string) (string, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(archivePath)
	}
	return extractFromTarGz(archivePath)
}

func extractFromTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		name := filepath.Base(header.Name)
		if name == "jend" {
			tmp, err := os.CreateTemp("", "jend-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(tmp, tr); err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", err
			}
			tmp.Close()
			if err := os.Chmod(tmp.Name(), 0755); err != nil {
				os.Remove(tmp.Name())
				return "", err
			}
			return tmp.Name(), nil
		}
	}
	return "", fmt.Errorf("jend binary not found in archive")
}

func extractFromZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "jend" || name == "jend.exe" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			tmp, err := os.CreateTemp("", "jend-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(tmp, rc); err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", err
			}
			tmp.Close()
			if err := os.Chmod(tmp.Name(), 0755); err != nil {
				os.Remove(tmp.Name())
				return "", err
			}
			return tmp.Name(), nil
		}
	}
	return "", fmt.Errorf("jend binary not found in archive")
}

func replaceBinary(currentPath, newPath string) error {
	// Rename current binary to .old (backup)
	backupPath := currentPath + ".old"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Copy new binary to current path
	src, err := os.Open(newPath)
	if err != nil {
		// Restore backup
		os.Rename(backupPath, currentPath)
		return fmt.Errorf("cannot open new binary: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		// Restore backup
		os.Rename(backupPath, currentPath)
		return fmt.Errorf("cannot write binary: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		// Restore backup
		os.Remove(currentPath)
		os.Rename(backupPath, currentPath)
		return fmt.Errorf("copy failed: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)
	return nil
}
