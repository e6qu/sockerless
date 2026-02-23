package core

import (
	"fmt"
	"net/http"
	"os"
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

	// Check duplicate
	if _, ok := s.Store.Volumes.Get(name); ok {
		// Docker returns the existing volume (idempotent)
		v, _ := s.Store.Volumes.Get(name)
		WriteJSON(w, http.StatusCreated, v)
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

	vol := api.Volume{
		Name:       name,
		Driver:     driver,
		Mountpoint: fmt.Sprintf("/var/lib/sockerless/volumes/%s/_data", name),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Labels:     labels,
		Scope:      "local",
		Options:    req.DriverOpts,
	}

	// For backends with real processes, create a host directory for volume data
	if !s.Drivers.ProcessLifecycle.IsSynthetic("") {
		dir, err := os.MkdirTemp("", "vol-"+name+"-*")
		if err == nil {
			s.Store.VolumeDirs.Store(name, dir)
			vol.Mountpoint = dir
		}
	}

	s.Store.Volumes.Put(name, vol)
	WriteJSON(w, http.StatusCreated, vol)
}

func (s *BaseServer) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	var vols []*api.Volume
	for _, v := range s.Store.Volumes.List() {
		if !MatchVolumeFilters(v, filters) {
			continue
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
	if !s.Store.Volumes.Delete(name) {
		WriteError(w, &api.NotFoundError{Resource: "volume", ID: name})
		return
	}
	if dir, ok := s.Store.VolumeDirs.LoadAndDelete(name); ok {
		os.RemoveAll(dir.(string))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	// Delete volumes that have no containers using them
	for _, v := range s.Store.Volumes.List() {
		inUse := false
		for _, c := range s.Store.Containers.List() {
			for _, m := range c.Mounts {
				if m.Name == v.Name {
					inUse = true
					break
				}
			}
			if inUse {
				break
			}
		}
		if !inUse {
			s.Store.Volumes.Delete(v.Name)
			if dir, ok := s.Store.VolumeDirs.LoadAndDelete(v.Name); ok {
				os.RemoveAll(dir.(string))
			}
			deleted = append(deleted, v.Name)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	WriteJSON(w, http.StatusOK, api.VolumePruneResponse{
		VolumesDeleted: deleted,
		SpaceReclaimed: 0,
	})
}
