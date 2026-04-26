package core

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/sockerless/api"
)

// handleContainerCommit creates a new image from a container's current state.
func (s *BaseServer) handleContainerCommit(w http.ResponseWriter, r *http.Request) {
	req := api.ContainerCommitRequest{
		Container: r.URL.Query().Get("container"),
		Repo:      r.URL.Query().Get("repo"),
		Tag:       r.URL.Query().Get("tag"),
		Comment:   r.URL.Query().Get("comment"),
		Author:    r.URL.Query().Get("author"),
		Pause:     r.URL.Query().Get("pause") != "false" && r.URL.Query().Get("pause") != "0",
	}

	if changesParam := r.URL.Query().Get("changes"); changesParam != "" {
		req.Changes = strings.Split(changesParam, "\n")
	}

	// Apply optional body overrides
	if r.Body != nil {
		defer r.Body.Close()
		var overrides struct {
			Cmd        []string `json:"Cmd,omitempty"`
			Entrypoint []string `json:"Entrypoint,omitempty"`
			Env        []string `json:"Env,omitempty"`
			WorkingDir string   `json:"WorkingDir,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&overrides); err != nil && err != io.EOF {
			WriteError(w, &api.InvalidParameterError{Message: "invalid commit config: " + err.Error()})
			return
		}
		if len(overrides.Cmd) > 0 || len(overrides.Entrypoint) > 0 || len(overrides.Env) > 0 || overrides.WorkingDir != "" {
			cfg := &api.ContainerConfig{}
			cfg.Cmd = overrides.Cmd
			cfg.Entrypoint = overrides.Entrypoint
			cfg.Env = overrides.Env
			cfg.WorkingDir = overrides.WorkingDir
			req.Config = cfg
		}
	}

	c, _ := s.ResolveContainerAuto(r.Context(), req.Container)
	if c.ID == "" {
		c.ID = req.Container
	}
	dctx := DriverContext{
		Ctx:       r.Context(),
		Container: c,
		Backend:   s.Desc.Driver,
		Logger:    s.Logger,
	}
	imageID, err := s.Typed.Commit.Commit(dctx, CommitOptions{
		Author:  req.Author,
		Comment: req.Comment,
		Repo:    req.Repo,
		Tag:     req.Tag,
		Pause:   req.Pause,
		Changes: req.Changes,
		Config:  req.Config,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, &api.ContainerCommitResponse{ID: imageID})
}

// parseJSONOrShell tries to parse value as a JSON array, falling back to shell form.
func parseJSONOrShell(value string) []string {
	var result []string
	if err := json.Unmarshal([]byte(value), &result); err == nil {
		return result
	}
	return []string{"/bin/sh", "-c", value}
}
