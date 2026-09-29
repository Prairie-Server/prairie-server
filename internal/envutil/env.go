package envutil

import (
	"os"
	"strings"
)

// Brand prefixes for server settings. Prairie is a rebrand of Silo, so every
// SILO_* setting is also readable as PRAIRIE_*; the two spellings name one
// setting rather than two.
const (
	prairiePrefix = "PRAIRIE_"
	siloPrefix    = "SILO_"
)

// LookupEnv is os.LookupEnv with the brand alias applied. For a SILO_X or
// PRAIRIE_X name, PRAIRIE_X wins when it is set to a non-empty value and SILO_X
// is the fallback, whichever spelling the caller asked for. An empty PRAIRIE_X
// does not mask SILO_X: compose files commonly pass "PRAIRIE_X=${PRAIRIE_X:-}"
// through, and that must not erase a value an older .env still sets as SILO_X.
// Every other name is looked up unchanged.
//
// Code keeps reading the names it always read (upstream Silo's SILO_X), and
// operators configure PRAIRIE_X; this is the one place that reconciles them.
func LookupEnv(name string) (string, bool) {
	suffix, ok := strings.CutPrefix(name, siloPrefix)
	if !ok {
		suffix, ok = strings.CutPrefix(name, prairiePrefix)
	}
	if !ok || suffix == "" {
		return os.LookupEnv(name)
	}
	prairieValue, prairieSet := os.LookupEnv(prairiePrefix + suffix)
	if prairieValue != "" {
		return prairieValue, true
	}
	if siloValue, siloSet := os.LookupEnv(siloPrefix + suffix); siloSet {
		return siloValue, true
	}
	return prairieValue, prairieSet
}

// Getenv is os.Getenv with the brand alias applied (see LookupEnv).
func Getenv(name string) string {
	value, _ := LookupEnv(name)
	return value
}

// FirstNonEmpty returns the first of keys whose value, trimmed, is non-empty.
// Each key is read through Getenv, so brand spellings alias.
func FirstNonEmpty(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
