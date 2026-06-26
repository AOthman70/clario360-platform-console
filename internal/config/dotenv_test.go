package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestParseDotEnv covers the supported .env grammar: comments, blanks, the
// export prefix, quote stripping, first-'=' splitting, and that '#' inside an
// unquoted value is preserved (not treated as an inline comment).
func TestParseDotEnv(t *testing.T) {
	const input = `
# a comment
PLATFORM_HTTP_ADDR=:8080

export PLATFORM_JWT_ISSUER=clario-gateway
QUOTED_DOUBLE="hello world"
QUOTED_SINGLE='single value'
DSN=postgres://u:p#ass@localhost:5432/db?sslmode=disable
SPACED =  trimmed
`
	got, err := parseDotEnv(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseDotEnv: %v", err)
	}

	want := map[string]string{
		"PLATFORM_HTTP_ADDR": ":8080",
		"PLATFORM_JWT_ISSUER": "clario-gateway",
		"QUOTED_DOUBLE":       "hello world",
		"QUOTED_SINGLE":       "single value",
		"DSN":                 "postgres://u:p#ass@localhost:5432/db?sslmode=disable",
		"SPACED":              "trimmed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDotEnv mismatch:\n got  %#v\n want %#v", got, want)
	}
}

func TestParseDotEnv_MissingEquals(t *testing.T) {
	if _, err := parseDotEnv(strings.NewReader("NOEQUALS")); err == nil {
		t.Fatal("expected error for a line without '='")
	}
}

// TestLoadDotEnv_OSPrecedence is the core contract: a key already present in the
// OS environment is left untouched; a key only in the file is seeded from it.
func TestLoadDotEnv_OSPrecedence(t *testing.T) {
	const osKey = "CFG_TEST_FROM_OS"
	const fileKey = "CFG_TEST_FROM_FILE"

	// Present in the OS env with a real value — the file must not override it.
	t.Setenv(osKey, "os-wins")
	// Ensure the file-only key starts absent and is cleaned up afterwards
	// (loadDotEnv uses os.Setenv, which outlives the test otherwise).
	os.Unsetenv(fileKey)
	t.Cleanup(func() { os.Unsetenv(fileKey) })

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := osKey + "=from-file\n" + fileKey + "=from-file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	if got := os.Getenv(osKey); got != "os-wins" {
		t.Errorf("%s = %q, want %q — OS env must take precedence", osKey, got, "os-wins")
	}
	if got := os.Getenv(fileKey); got != "from-file" {
		t.Errorf("%s = %q, want %q — file value should seed a missing key", fileKey, got, "from-file")
	}
}

// TestLoadDotEnv_MissingFileOK confirms an absent .env is not an error.
func TestLoadDotEnv_MissingFileOK(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Errorf("loadDotEnv on missing file = %v, want nil", err)
	}
}
