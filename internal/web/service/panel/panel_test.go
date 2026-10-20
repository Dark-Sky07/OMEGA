package panel

import (
	"strings"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v2.9.4", "2.9.3", true},
		{"v2.10.0", "2.9.9", true},
		{"v2.9.3", "2.9.3", false},
		{"v2.9.2", "2.9.3", false},
		{"v3.0.0", "2.9.3", true},
		// PHOENIX ships "-phoenix" suffixed tags; the suffix must not break the
		// numeric comparison.
		{"v3.3.4-phoenix", "3.3.4-phoenix", false},
		{"v3.3.4-phoenix", "3.3.3-phoenix", true},
		{"v3.3.3-phoenix", "3.3.4-phoenix", false},
		{"v3.3.10-phoenix", "3.3.9-phoenix", true},
		{"v5.0.0-phoenix", "4.7.3-phoenix", true},
		{"v5.0.0-phoenix", "5.0.0-phoenix", false},
		{"v5.0.1-phoenix", "5.0.0-phoenix", true},
	}

	for _, tc := range cases {
		if got := isNewerVersion(tc.latest, tc.current); got != tc.want {
			t.Fatalf("isNewerVersion(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestCompareVersionStringsRejectsUnexpectedFormats(t *testing.T) {
	if _, ok := compareVersionStrings("latest", "2.9.3"); ok {
		t.Fatal("expected non-semver latest tag to be rejected")
	}
	if _, ok := compareVersionStrings("v2.9", "2.9.3"); ok {
		t.Fatal("expected short version to be rejected")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/usr/bin/curl"); got != "'/usr/bin/curl'" {
		t.Fatalf("unexpected quote result: %s", got)
	}
	if got := shellQuote("/tmp/a'b"); got != "'/tmp/a'\\''b'" {
		t.Fatalf("unexpected quote result with single quote: %s", got)
	}
}

// TestPanelUpdaterURLUsesCanonicalRepoCase guards against the v5.0.2/v5.0.3
// bug where a lower-case repo name made raw.githubusercontent.com return 404
// for the in-panel updater download (GitHub's raw service is case-sensitive).
func TestPanelUpdaterURLUsesCanonicalRepoCase(t *testing.T) {
	if phoenixRepo != "Dark-Sky07/PHOENIX" {
		t.Fatalf("phoenixRepo must be the canonical Dark-Sky07/PHOENIX, got %q", phoenixRepo)
	}
	if !strings.HasPrefix(panelUpdaterURL, "https://raw.githubusercontent.com/Dark-Sky07/PHOENIX/") {
		t.Fatalf("panelUpdaterURL must use the canonical repo case, got %q", panelUpdaterURL)
	}
}
