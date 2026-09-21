package domain

// Registry is the set of workspaces bosk has created. It is the on-disk
// document too, so the JSON shape is part of the file format.
type Registry struct {
	Workspaces []Workspace `json:"workspaces"`
}

// Get returns a workspace by name.
func (r *Registry) Get(name string) (Workspace, bool) {
	for _, w := range r.Workspaces {
		if w.Name == name {
			return w, true
		}
	}
	return Workspace{}, false
}

// Upsert replaces or appends a workspace record.
func (r *Registry) Upsert(ws Workspace) {
	for i, w := range r.Workspaces {
		if w.Name == ws.Name {
			r.Workspaces[i] = ws
			return
		}
	}
	r.Workspaces = append(r.Workspaces, ws)
}

// Remove deletes a workspace record by name.
func (r *Registry) Remove(name string) bool {
	for i, w := range r.Workspaces {
		if w.Name == name {
			r.Workspaces = append(r.Workspaces[:i], r.Workspaces[i+1:]...)
			return true
		}
	}
	return false
}
