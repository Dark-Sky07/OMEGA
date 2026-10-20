package panel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/config"
	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/web/global"
	"github.com/Dark-Sky07/phoenix/internal/web/service"
)

// PanelService provides business logic for panel management operations.
// It handles panel restart, updates, and system-level panel controls.
type PanelService struct{}

// PanelUpdateInfo contains the current and latest available panel versions.
type PanelUpdateInfo struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

// phoenixRepo is the canonical PHOENIX project. In-panel updates must only
// ever pull from this repository — never from upstream panel builds.
// NOTE: the case must match the repository name exactly. GitHub's API is
// case-insensitive, but raw.githubusercontent.com is NOT — a lower-case repo
// segment makes the installer download return 404 (this exact bug shipped in
// v5.0.2/v5.0.3 and broke the in-panel update button).
const phoenixRepo = "Dark-Sky07/PHOENIX"

const (
	// panelUpdaterURL is the script the in-panel update button downloads and
	// runs. It is the very same installer the CLI uses (`x-ui.sh update` →
	// install-phoenix.sh), which resolves the latest PHOENIX release and
	// restarts the panel — so an in-panel update can never install upstream
	// builds.
	panelUpdaterURL      = "https://raw.githubusercontent.com/" + phoenixRepo + "/main/install-phoenix.sh"
	maxPanelUpdaterBytes = 2 << 20

	// panelUpdateInfoTTL caches the GitHub release lookup so multiple UI
	// surfaces (dashboard card, sidebar badge, update modal) and frequent
	// page reloads do not exhaust the GitHub API rate limit.
	panelUpdateInfoTTL = 5 * time.Minute
)

var (
	panelUpdateInfoMu        sync.Mutex
	panelUpdateInfoCache     *PanelUpdateInfo
	panelUpdateInfoFetchedAt time.Time
)

func (s *PanelService) RestartPanel(delay time.Duration) error {
	go func() {
		time.Sleep(delay)
		if global.TriggerRestart() {
			return
		}
		if runtime.GOOS == "windows" {
			logger.Error("panel restart: no restart hook registered (SIGHUP unsupported on Windows)")
			return
		}
		p, err := os.FindProcess(syscall.Getpid())
		if err != nil {
			logger.Error("panel restart: FindProcess failed:", err)
			return
		}
		if err := p.Signal(syscall.SIGHUP); err != nil {
			logger.Error("failed to send SIGHUP signal:", err)
		}
	}()
	return nil
}

// GetUpdateInfo checks the PHOENIX repository for the latest panel release.
// Results are cached for a short window so that the dashboard, sidebar and
// the update modal can each ask without hammering the GitHub API.
func (s *PanelService) GetUpdateInfo(force bool) (*PanelUpdateInfo, error) {
	panelUpdateInfoMu.Lock()
	defer panelUpdateInfoMu.Unlock()
	if !force && panelUpdateInfoCache != nil && time.Since(panelUpdateInfoFetchedAt) < panelUpdateInfoTTL {
		return panelUpdateInfoCache, nil
	}
	latest, err := fetchLatestPanelVersion()
	if err != nil {
		return nil, err
	}
	current := config.GetVersion()
	info := &PanelUpdateInfo{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: isNewerVersion(latest, current),
	}
	panelUpdateInfoCache = info
	panelUpdateInfoFetchedAt = time.Now()
	return info, nil
}

// StartUpdate starts the official updater outside of the current web request.
func (s *PanelService) StartUpdate() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("panel web update is supported only on Linux installations")
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		return fmt.Errorf("bash is required to run the panel updater: %w", err)
	}

	scriptPath, err := downloadPanelUpdater()
	if err != nil {
		return err
	}

	mainFolder, serviceFolder := resolveUpdateFolders()
	updateScript := fmt.Sprintf("set -e; trap 'rm -f %s' EXIT; %s %s", shellQuote(scriptPath), shellQuote(bash), shellQuote(scriptPath))

	if systemdRun, err := exec.LookPath("systemd-run"); err == nil {
		unitName := fmt.Sprintf("x-ui-web-update-%d", time.Now().Unix())
		cmd := exec.Command(systemdRun,
			"--unit", unitName,
			"--setenv", "XUI_MAIN_FOLDER="+mainFolder,
			"--setenv", "XUI_SERVICE="+serviceFolder,
			bash, "-lc", updateScript,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			output := strings.TrimSpace(string(out))
			if !strings.Contains(output, "System has not been booted with systemd") &&
				!strings.Contains(output, "Failed to connect to bus") {
				_ = os.Remove(scriptPath)
				return fmt.Errorf("failed to start panel update job: %w: %s", err, output)
			}
			logger.Warning("systemd-run is unavailable, falling back to detached update process:", output)
		} else {
			logger.Infof("started panel update job via systemd-run unit %s", unitName)
			return nil
		}
	}

	cmd := exec.Command(bash, "-lc", updateScript)
	cmd.Env = append(os.Environ(),
		"XUI_MAIN_FOLDER="+mainFolder,
		"XUI_SERVICE="+serviceFolder,
	)
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("failed to start panel update job: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		logger.Warning("failed to release panel update process:", err)
	}
	logger.Infof("started panel update job with pid %d", cmd.Process.Pid)
	return nil
}

func downloadPanelUpdater() (string, error) {
	client := (&service.SettingService{}).NewProxiedHTTPClient(15 * time.Second)
	resp, err := client.Get(panelUpdaterURL)
	if err != nil {
		return "", fmt.Errorf("download panel updater: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download panel updater: unexpected HTTP %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "phoenix-update-*.sh")
	if err != nil {
		return "", err
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()

	n, err := io.Copy(file, io.LimitReader(resp.Body, maxPanelUpdaterBytes+1))
	if err != nil {
		return "", fmt.Errorf("write panel updater: %w", err)
	}
	if n > maxPanelUpdaterBytes {
		return "", fmt.Errorf("panel updater exceeds %d bytes", maxPanelUpdaterBytes)
	}
	if err := file.Chmod(0700); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func fetchLatestPanelVersion() (string, error) {
	client := (&service.SettingService{}).NewProxiedHTTPClient(10 * time.Second)
	// PHOENIX ships pre-releases, so /releases/latest would 404 while the
	// newest tag is a pre-release. Use the releases list instead (includes
	// pre-releases, newest first) — the same source of truth
	// install-phoenix.sh uses to resolve the update target.
	resp, err := client.Get("https://api.github.com/repos/" + phoenixRepo + "/releases?per_page=10")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	var releases []service.Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}
	if len(releases) == 0 || releases[0].TagName == "" {
		return "", fmt.Errorf("no releases found for %s", phoenixRepo)
	}
	return releases[0].TagName, nil
}

func resolveUpdateFolders() (string, string) {
	mainFolder := os.Getenv("XUI_MAIN_FOLDER")
	if mainFolder == "" {
		if exePath, err := os.Executable(); err == nil {
			mainFolder = filepath.Dir(exePath)
		}
	}
	if mainFolder == "" {
		mainFolder = "/usr/local/x-ui"
	}

	serviceFolder := os.Getenv("XUI_SERVICE")
	if serviceFolder == "" {
		serviceFolder = "/etc/systemd/system"
	}
	return mainFolder, serviceFolder
}

func isNewerVersion(latest string, current string) bool {
	cmp, ok := compareVersionStrings(latest, current)
	if !ok {
		return normalizeVersionTag(latest) != normalizeVersionTag(current)
	}
	return cmp > 0
}

func compareVersionStrings(a string, b string) (int, bool) {
	aParts, okA := parseVersionParts(a)
	bParts, okB := parseVersionParts(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range len(aParts) {
		if aParts[i] > bParts[i] {
			return 1, true
		}
		if aParts[i] < bParts[i] {
			return -1, true
		}
	}
	return 0, true
}

func parseVersionParts(version string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(normalizeVersionTag(version), ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func normalizeVersionTag(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	// Drop pre-release/build suffixes (e.g. "5.0.0-phoenix" → "5.0.0") so
	// PHOENIX tags compare numerically against each other.
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	return v
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
