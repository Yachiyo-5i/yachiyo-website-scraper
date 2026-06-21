package buildinfo

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesVersionFile(t *testing.T) {
	raw, err := os.ReadFile("version")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(raw))
	if want == "" {
		want = "0.0.0"
	}
	if got := Version(); got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}

func TestVersionDefaultsWhenEmbeddedVersionIsBlank(t *testing.T) {
	old := embeddedVersion
	t.Cleanup(func() {
		embeddedVersion = old
	})

	embeddedVersion = " \n\t "
	if got := Version(); got != "0.0.0" {
		t.Fatalf("Version() = %q, want %q", got, "0.0.0")
	}

	embeddedVersion = " 1.2.3 \n"
	if got := Version(); got != "1.2.3" {
		t.Fatalf("Version() = %q, want %q", got, "1.2.3")
	}
}
