// Package config loads JSON configuration files, replacing the
// Microsoft.Extensions configuration binder. The file path can be overridden via
// a flag or the NTUNL_CONFIG environment variable.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads path and unmarshals it into v. If the NTUNL_CONFIG env var is set it
// takes precedence over path.
func Load(path string, v any) error {
	if env := os.Getenv("NTUNL_CONFIG"); env != "" {
		path = env
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse config %q: %w", path, err)
	}
	return nil
}
