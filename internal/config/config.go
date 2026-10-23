package config

import (
	"os"
	"strconv"
	"strings"
)

func String(name string, fallback string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return fallback
}

// Strings splits a comma-separated env var as-is, without trimming or dropping items.
// Returns nil when the env var is unset or empty.
func Strings(name string) []string {
	s := os.Getenv(name)
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func Int(name string, fallback int) int {
	if s := os.Getenv(name); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return fallback
}
