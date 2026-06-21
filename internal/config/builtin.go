package config

import (
	"fmt"
	"sort"
	"strings"

	"yachiyo-website-scraper/configs"
)

func LoadBuiltin(site string) (*Config, error) {
	data, err := ReadBuiltin(site)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func ReadBuiltin(site string) ([]byte, error) {
	name := normalizeBuiltinName(site)
	if name == "" {
		return nil, fmt.Errorf("site name is required")
	}
	data, err := configs.Files.ReadFile(name + ".yml")
	if err != nil {
		return nil, fmt.Errorf("builtin site %q not found", site)
	}
	return data, nil
}

func BuiltinSites() ([]string, error) {
	entries, err := configs.Files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	sites := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		sites = append(sites, strings.TrimSuffix(entry.Name(), ".yml"))
	}
	sort.Strings(sites)
	return sites, nil
}

func normalizeBuiltinName(site string) string {
	name := strings.TrimSpace(site)
	name = strings.TrimSuffix(name, ".yml")
	name = strings.TrimSuffix(name, ".yaml")
	if strings.Contains(name, "/") || strings.Contains(name, `\`) || name == "." || name == ".." {
		return ""
	}
	return name
}
