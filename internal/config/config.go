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

func Strings(name string) []string {
	s := os.Getenv(name)
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func Bool(name string, fallback bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(name)); err == nil {
		return v
	}
	return fallback
}

func Int(name string, fallback int) int {
	if s := os.Getenv(name); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return fallback
}
