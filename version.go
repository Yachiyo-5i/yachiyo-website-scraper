package buildinfo

import (
	_ "embed"
	"strings"
)

//go:embed version
var embeddedVersion string

// Version returns the version compiled into the binary.
func Version() string {
	version := strings.TrimSpace(embeddedVersion)
	if version == "" {
		return "0.0.0"
	}
	return version
}
