package core

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sockerless/api"
)

func (s *BaseServer) handleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req api.VolumeCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	name := req.Name
	if name == "" {
		name = GenerateID()[:12]
	}

	// Check duplicate — Docker returns 200 for existing, 201 for new
	if _, ok := s.Store.Volumes.Get(name); ok {
		v, _ := s.Store.Volumes.Get(name)
		WriteJSON(w, http.StatusOK, v)
		return
	}

	driver := req.Driver
	if driver == "" {
		driver = "local"
	}

	labels := req.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	options := req.DriverOpts
	if options == nil {
		options = make(map[string]string)
	}

	vol := api.Volume{
		Name:       name,
		Driver:     driver,
		Mountpoint: fmt.Sprintf("/var/lib/sockerless/volumes/%s/_data", name),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Labels:     labels,
		Scope:      "local",
		Options:    options,
	}

	dir, err := os.MkdirTemp("", "vol-"+name+"-*")
	if err == nil {
		s.Store.VolumeDirs.Store(name, dir)
		vol.Mountpoint = dir
	}

	s.Store.Volumes.Put(name, vol)

	s.emitEvent("volume", "create", name, map[string]string{
		"driver": driver,
	})

	WriteJSON(w, http.StatusCreated, vol)
}

func (s *BaseServer) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	// Build in-use volume names for dangling filter
	var inUseNames map[string]bool
	if _, hasDangling := filters["dangling"]; hasDangling {
		inUseNames = make(map[string]bool)
		for _, c := range s.Store.Containers.List() {
			for _, m := range c.Mounts {
				if m.Name != "" {
					inUseNames[m.Name] = true
				}
			}
			for _, bind := range c.HostConfig.Binds {
				parts := strings.SplitN(bind, ":", 3)
				if len(parts) >= 2 && !filepath.IsAbs(parts[0]) {
					inUseNames[parts[0]] = true
				}
			}
		}
	}

	var vols []*api.Volume
	for _, v := range s.Store.Volumes.List() {
		if !MatchVolumeFilters(v, filters) {
			continue
		}
		// Apply dangling filter
		if danglingVals, ok := filters["dangling"]; ok {
			wantDangling := danglingVals[0] == "true" || danglingVals[0] == "1"
			isDangling := !inUseNames[v.Name]
			if wantDangling != isDangling {
				continue
			}
		}
		v := v
		vols = append(vols, &v)
	}
	if vols == nil {
		vols = []*api.Volume{}
	}
	WriteJSON(w, http.StatusOK, api.VolumeListResponse{
		Volumes:  vols,
		Warnings: []string{},
	})
}

func (s *BaseServer) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	vol, ok := s.Store.Volumes.Get(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "volume", ID: name})
		return
	}
	WriteJSON(w, http.StatusOK, vol)
}

func (s *BaseServer) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"

	if err := s.self.VolumeRemove(name, force); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	resp, err := s.self.VolumePrune(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}
