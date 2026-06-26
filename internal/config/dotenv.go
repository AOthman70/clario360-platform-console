package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// loadDotEnv reads KEY=VALUE pairs from the file at path and sets each in the
// process environment ONLY when that key is not already present. This gives the
// OS environment precedence over the file: a value exported in the shell — or
// injected by the container/orchestrator — always wins, and the .env file is the
// fallback for local development. A missing file is not an error (the .env is
// optional; production runs purely on the real environment).
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional.
		}
		return err
	}
	defer f.Close()

	vars, err := parseDotEnv(f)
	if err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	for k, v := range vars {
		if _, ok := os.LookupEnv(k); ok {
			continue // OS environment takes precedence; never override it.
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// parseDotEnv parses a minimal .env format:
//   - blank lines and lines beginning with '#' are ignored;
//   - each remaining line is KEY=VALUE, split on the first '=';
//   - a leading "export " is tolerated;
//   - one matching pair of surrounding single or double quotes is stripped.
//
// Inline comments are intentionally NOT supported: an unquoted value is taken
// verbatim so that values legitimately containing '#' (DSNs, passwords) survive.
func parseDotEnv(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")

		key, val, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: missing '=' in %q", line, raw)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", line)
		}
		out[key] = unquote(strings.TrimSpace(val))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// unquote strips a single matching pair of surrounding single or double quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
