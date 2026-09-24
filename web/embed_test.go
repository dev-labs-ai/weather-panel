package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestStaticHoldsHtmx4(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(Static, "js/htmx.min.js")
	if err != nil {
		t.Fatalf("read js/htmx.min.js: %v", err)
	}
	if !strings.Contains(string(data), `version="4.0.0"`) {
		t.Error("js/htmx.min.js is not htmx 4.0.0")
	}
}

func TestAssetPathCarriesAContentVersion(t *testing.T) {
	t.Parallel()

	got := AssetPath("js/htmx.min.js")
	if !regexp.MustCompile(`^/static/js/htmx\.min\.js\?v=[0-9a-f]{12}$`).MatchString(got) {
		t.Errorf("AssetPath() = %q, want /static/js/htmx.min.js?v=<12 hex digits>", got)
	}
	if again := AssetPath("js/htmx.min.js"); again != got {
		t.Errorf("AssetPath() = %q then %q, want a stable version", got, again)
	}
}

func TestAssetPathOfAMissingFile(t *testing.T) {
	t.Parallel()

	if got := AssetPath("js/missing.js"); got != "/static/js/missing.js" {
		t.Errorf("AssetPath() = %q, want /static/js/missing.js", got)
	}
}
