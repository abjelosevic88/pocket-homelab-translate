package server

import (
	"net/http"
	"net/url"
	"strings"
)

// ---- the other Pocket apps ----

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

// handleBrowseFiles lists a folder in Pocket Files (to pick a document).
func (s *Server) handleBrowseFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appFiles, scopeFilesRead)
	if err != nil {
		s.appErr(w, appFiles, err)
		return
	}
	var out struct {
		Entries []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
			Size  int64  `json:"size"`
			Mime  string `json:"mime"`
		} `json:"entries"`
	}
	p := strings.Trim(r.URL.Query().Get("path"), "/")
	if err := sib.client.JSON(ctx, http.MethodGet, "/api/v1/fs/list", url.Values{"space": {"home"}, "path": {p}}, nil, &out); err != nil {
		s.appErr(w, appFiles, err)
		return
	}
	entries := []map[string]any{}
	for _, e := range out.Entries {
		entries = append(entries, map[string]any{"name": e.Name, "path": e.Path, "dir": e.IsDir, "size": e.Size, "mime": e.Mime})
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "entries": entries})
}

// handleSearchDocuments finds documents in Pocket Documents (to pick one).
func (s *Server) handleSearchDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appDocuments, scopeDocumentsRead)
	if err != nil {
		s.appErr(w, appDocuments, err)
		return
	}
	var out struct {
		Results []struct {
			ID      int64  `json:"id"`
			Title   string `json:"title"`
			Created string `json:"created"`
			Mime    string `json:"mime"`
		} `json:"results"`
	}
	if err := sib.client.JSON(ctx, http.MethodGet, "/api/v1/documents", url.Values{"q": {r.URL.Query().Get("q")}, "limit": {"25"}}, nil, &out); err != nil {
		s.appErr(w, appDocuments, err)
		return
	}
	list := []map[string]any{}
	for _, d := range out.Results {
		list = append(list, map[string]any{"id": d.ID, "title": d.Title, "created": d.Created, "mime": d.Mime})
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": list})
}
