package domain

import (
	"fmt"
	"strings"
)

var branchToName = strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")

// NameFromBranch derives a directory-safe workspace name from a branch name.
func NameFromBranch(branch string) string {
	return branchToName.Replace(branch)
}

// ValidateWorkspaceName rejects names that would escape the workspace root.
func ValidateWorkspaceName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid workspace name %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("workspace name must not contain path separators")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid workspace name %q", name)
	}
	return nil
}
