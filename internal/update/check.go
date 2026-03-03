package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/darkprince558/jend/internal/config"
	"golang.org/x/mod/semver"
)

const (
	githubRepo   = "darkprince558/JEND"
	githubAPIURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// CheckBackgroundAsync optionally checks for a new release in a background goroutine
// if it has been more than 24 hours since the last check. It will save the latest
// version found to the local config file for future runs to display.
func CheckBackgroundAsync() {
	go func() {
		cfg, err := config.Load()
		if err != nil {
			return
		}

		// Only check once every 24 hours
		now := time.Now().Unix()
		if now-cfg.LastUpdateCheck < 86400 {
			return
		}

		// Perform API check with a short timeout so we don't hold up anything if network is weird
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(githubAPIURL)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var release ghRelease
			if err := json.NewDecoder(resp.Body).Decode(&release); err == nil {
				cfg.LatestVersion = release.TagName
			}
		}

		// Update timestamp regardless of success/fail to avoid spamming the API
		cfg.LastUpdateCheck = now
		_ = config.Save(cfg)
	}()
}

// GetUpdateNotice returns an update notice string if a new version is available, relative to the current version.
// If no update is available or the current version is "dev", returns an empty string.
func GetUpdateNotice(currentVersion string) string {
	if currentVersion == "dev" {
		return ""
	}

	cfg, err := config.Load()
	if err != nil || cfg.LatestVersion == "" {
		return ""
	}

	// Normalize
	current := currentVersion
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}
	latest := cfg.LatestVersion
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}

	// Very simple check: if the latest version from our cache is different, notify.
	// Since we only query /releases/latest, it shouldn't go backwards.
	if semver.Compare(latest, current) > 0 {
		return fmt.Sprintf("\n  \033[36m\033[1mA new version of JEND is available! (%s)\033[0m\n  Run \033[1mjend update\033[0m to install it.\n", latest)
	}

	return ""
}
