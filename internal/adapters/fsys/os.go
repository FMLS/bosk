// Package fsys implements the filesystem port against the real filesystem.
package fsys

import "os"

const dirPerm = 0o755

// OS implements the filesystem port using the os package.
type OS struct{}

// New returns a filesystem backed by the OS.
func New() OS { return OS{} }

// Exists reports whether path exists. A path that cannot be stated for any
// other reason returns the error, so callers can tell "absent" from "broken".
func (OS) Exists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsDir reports whether path exists and is a directory.
func (OS) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (OS) MkdirAll(path string) error { return os.MkdirAll(path, dirPerm) }

func (OS) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

func (OS) RemoveAll(path string) error { return os.RemoveAll(path) }

// ListDirs returns the names of the directory entries that are directories.
func (OS) ListDirs(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ListEntries returns the names of every entry in a directory.
func (OS) ListEntries(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func (OS) Getwd() (string, error) { return os.Getwd() }
