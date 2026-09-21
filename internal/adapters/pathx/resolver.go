// Package pathx resolves user-supplied paths into comparable absolute paths.
// It is a leaf adapter: keeping it out of the git and registry packages is what
// lets those two stay independent of each other.
package pathx

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolver implements the path port using the real filesystem.
type Resolver struct{}

// New returns a resolver backed by the OS.
func New() Resolver { return Resolver{} }

// Normalize returns a comparable absolute path, following symlinks when it can.
func (Resolver) Normalize(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// Resolve expands a leading ~ and makes the path absolute.
func (Resolver) Resolve(path string) (string, error) {
	return filepath.Abs(Expand(path))
}

// Expand replaces a leading ~ with the user's home directory.
func Expand(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
