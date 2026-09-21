package domain

import "strings"

// SelectRepos returns the requested repositories in request order, ignoring
// blanks and duplicates.
func SelectRepos(repos []Repo, names []string) ([]Repo, error) {
	index := make(map[string]Repo, len(repos))
	for _, r := range repos {
		index[r.Name] = r
	}
	var out []Repo
	var missing []string
	seen := make(map[string]bool)
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		r, ok := index[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		out = append(out, r)
	}
	if len(missing) > 0 {
		available := make([]string, 0, len(repos))
		for _, r := range repos {
			available = append(available, r.Name)
		}
		return nil, &MissingReposError{Names: missing, Available: available}
	}
	return out, nil
}

// MissingReposError lists unknown repository names.
type MissingReposError struct {
	Names     []string
	Available []string
}

func (e *MissingReposError) Error() string {
	msg := "unknown repositories: " + strings.Join(e.Names, ", ")
	if len(e.Available) > 0 {
		msg += " (available: " + strings.Join(e.Available, ", ") + ")"
	}
	return msg
}
