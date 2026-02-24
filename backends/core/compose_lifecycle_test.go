package core

import (
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestComposeCreateStartStopRemove(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	// 1. Create network
	netID := GenerateID()
	net := api.Network{
		Name:       "compose_default",
		ID:         netID,
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
		Driver:     "bridge",
		Containers: make(map[string]api.EndpointResource),
		Labels:     map[string]string{"com.docker.compose.project": "myapp"},
		Options:    make(map[string]string),
	}
	s.Store.Networks.Put(netID, net)
	s.Store.Networks.Put("compose_default", net)

	// 2. Create containers
	containers := []struct {
		id   string
		name string
	}{
		{"compose-web-1", "/myapp-web-1"},
		{"compose-db-1", "/myapp-db-1"},
	}

	for _, ct := range containers {
		c := api.Container{
			ID:      ct.id,
			Name:    ct.name,
			Created: time.Now().UTC().Format(time.RFC3339Nano),
			Config: api.ContainerConfig{
				Image:  "alpine:latest",
				Labels: map[string]string{"com.docker.compose.project": "myapp"},
			},
			State: api.ContainerState{Status: "created"},
			HostConfig: api.HostConfig{
				NetworkMode: "compose_default",
			},
			NetworkSettings: api.NetworkSettings{
				Networks: map[string]*api.EndpointSettings{
					"compose_default": {
						NetworkID: netID,
						IPAddress: "10.0.0.2",
					},
				},
			},
			Mounts: make([]api.MountPoint, 0),
		}
		s.Store.Containers.Put(ct.id, c)
		s.Store.ContainerNames.Put(ct.name, ct.id)

		// Add to network
		s.Store.Networks.Update(netID, func(n *api.Network) {
			n.Containers[ct.id] = api.EndpointResource{Name: ct.name}
		})
	}

	// 3. Start containers
	for _, ct := range containers {
		s.Store.Containers.Update(ct.id, func(c *api.Container) {
			c.State.Status = "running"
			c.State.Running = true
			c.State.Pid = 42
			c.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		})
	}

	// 4. Verify all running
	for _, ct := range containers {
		c, ok := s.Store.Containers.Get(ct.id)
		if !ok {
			t.Fatalf("container %s not found", ct.id)
		}
		if !c.State.Running {
			t.Errorf("container %s should be running", ct.id)
		}
	}

	// 5. Stop containers
	for _, ct := range containers {
		s.Store.ForceStopContainer(ct.id, 0)
	}

	// 6. Verify stopped
	for _, ct := range containers {
		c, _ := s.Store.Containers.Get(ct.id)
		if c.State.Running {
			t.Errorf("container %s should be stopped", ct.id)
		}
		if c.State.Status != "exited" {
			t.Errorf("container %s status = %q, want exited", ct.id, c.State.Status)
		}
	}

	// 7. Remove containers
	for _, ct := range containers {
		c, _ := s.Store.Containers.Get(ct.id)
		// Clean up network associations
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				s.Store.Networks.Update(ep.NetworkID, func(n *api.Network) {
					delete(n.Containers, ct.id)
				})
			}
		}
		s.Store.Containers.Delete(ct.id)
		s.Store.ContainerNames.Delete(ct.name)
	}

	// 8. Verify containers removed
	if s.Store.Containers.Len() != 0 {
		t.Errorf("expected 0 containers, got %d", s.Store.Containers.Len())
	}

	// 9. Remove network
	s.Store.Networks.Delete(netID)
	s.Store.Networks.Delete("compose_default")

	if s.Store.Networks.Len() != 0 {
		t.Errorf("expected 0 networks, got %d", s.Store.Networks.Len())
	}
}

func TestComposeVolumePersistenceAcrossRestarts(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	// Create a named volume
	vol := api.Volume{
		Name:       "myapp_data",
		Driver:     "local",
		Mountpoint: "/var/lib/docker/volumes/myapp_data/_data",
		Labels:     map[string]string{"com.docker.compose.project": "myapp"},
		Scope:      "local",
	}
	s.Store.Volumes.Put(vol.Name, vol)

	// Create and start container with volume
	c := api.Container{
		ID:      "vol-persist-1",
		Name:    "/myapp-db-1",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Image:  "postgres:15",
			Labels: make(map[string]string),
		},
		HostConfig: api.HostConfig{
			Binds: []string{"myapp_data:/var/lib/postgresql/data"},
		},
		State: api.ContainerState{
			Status:  "running",
			Running: true,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: []api.MountPoint{
			{
				Type:        "volume",
				Name:        "myapp_data",
				Source:       "/var/lib/docker/volumes/myapp_data/_data",
				Destination: "/var/lib/postgresql/data",
				RW:          true,
			},
		},
	}
	s.Store.Containers.Put(c.ID, c)
	s.Store.ContainerNames.Put(c.Name, c.ID)

	// Stop and remove container (simulating compose down)
	s.Store.ForceStopContainer(c.ID, 0)
	s.Store.Containers.Delete(c.ID)
	s.Store.ContainerNames.Delete(c.Name)

	// Volume should persist
	v, ok := s.Store.Volumes.Get("myapp_data")
	if !ok {
		t.Fatal("expected volume to persist after container removal")
	}
	if v.Name != "myapp_data" {
		t.Errorf("volume name = %q, want myapp_data", v.Name)
	}

	// Create new container with same volume (simulating compose up again)
	c2 := api.Container{
		ID:      "vol-persist-2",
		Name:    "/myapp-db-1",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Image:  "postgres:15",
			Labels: make(map[string]string),
		},
		HostConfig: api.HostConfig{
			Binds: []string{"myapp_data:/var/lib/postgresql/data"},
		},
		State: api.ContainerState{
			Status:  "running",
			Running: true,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: []api.MountPoint{
			{
				Type:        "volume",
				Name:        "myapp_data",
				Source:       "/var/lib/docker/volumes/myapp_data/_data",
				Destination: "/var/lib/postgresql/data",
				RW:          true,
			},
		},
	}
	s.Store.Containers.Put(c2.ID, c2)
	s.Store.ContainerNames.Put(c2.Name, c2.ID)

	// Verify new container is running with same volume
	got, ok := s.Store.Containers.Get("vol-persist-2")
	if !ok {
		t.Fatal("expected new container to exist")
	}
	if !got.State.Running {
		t.Error("expected new container to be running")
	}
	if len(got.Mounts) != 1 || got.Mounts[0].Name != "myapp_data" {
		t.Errorf("expected mount to reference myapp_data, got %v", got.Mounts)
	}
}

func TestComposeNetworkCleanupOnDown(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	// Create network with containers
	netID := GenerateID()
	net := api.Network{
		Name:       "myapp_default",
		ID:         netID,
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
		Driver:     "bridge",
		Containers: make(map[string]api.EndpointResource),
		Labels:     make(map[string]string),
		Options:    make(map[string]string),
	}
	s.Store.Networks.Put(netID, net)
	s.Store.Networks.Put("myapp_default", net)

	// Add two containers
	for i, id := range []string{"cleanup-1", "cleanup-2"} {
		c := api.Container{
			ID:   id,
			Name: "/" + id,
			Config: api.ContainerConfig{
				Image:  "alpine",
				Labels: make(map[string]string),
			},
			State: api.ContainerState{Status: "running", Running: true, Pid: 42},
			NetworkSettings: api.NetworkSettings{
				Networks: map[string]*api.EndpointSettings{
					"myapp_default": {NetworkID: netID, IPAddress: "10.0.0." + string(rune('2'+i))},
				},
			},
			Mounts: make([]api.MountPoint, 0),
		}
		s.Store.Containers.Put(id, c)
		s.Store.ContainerNames.Put(c.Name, id)

		s.Store.Networks.Update(netID, func(n *api.Network) {
			n.Containers[id] = api.EndpointResource{Name: c.Name}
		})
	}

	// Verify network has 2 containers
	n, _ := s.Store.Networks.Get(netID)
	if len(n.Containers) != 2 {
		t.Fatalf("expected 2 containers in network, got %d", len(n.Containers))
	}

	// Remove containers (simulating compose down)
	for _, id := range []string{"cleanup-1", "cleanup-2"} {
		c, _ := s.Store.Containers.Get(id)
		s.Store.ForceStopContainer(id, 0)
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				s.Store.Networks.Update(ep.NetworkID, func(n *api.Network) {
					delete(n.Containers, id)
				})
			}
		}
		s.Store.Containers.Delete(id)
		s.Store.ContainerNames.Delete(c.Name)
	}

	// Network should have 0 containers now
	n, _ = s.Store.Networks.Get(netID)
	if len(n.Containers) != 0 {
		t.Errorf("expected 0 containers in network after cleanup, got %d", len(n.Containers))
	}

	// Remove network
	s.Store.Networks.Delete(netID)
	s.Store.Networks.Delete("myapp_default")

	if s.Store.Networks.Len() != 0 {
		t.Errorf("expected 0 networks after cleanup, got %d", s.Store.Networks.Len())
	}
}

func TestComposeContainerNameReuseAfterRemove(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	name := "/myapp-web-1"

	// Create first container
	c1 := api.Container{
		ID:   "reuse-old",
		Name: name,
		Config: api.ContainerConfig{
			Image:  "nginx:1.24",
			Labels: make(map[string]string),
		},
		State: api.ContainerState{Status: "running", Running: true},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	s.Store.Containers.Put(c1.ID, c1)
	s.Store.ContainerNames.Put(name, c1.ID)

	// Stop and remove
	s.Store.ForceStopContainer(c1.ID, 0)
	s.Store.Containers.Delete(c1.ID)
	s.Store.ContainerNames.Delete(name)

	// Verify name is available
	if _, exists := s.Store.ContainerNames.Get(name); exists {
		t.Fatal("expected name to be available after removal")
	}

	// Create second container with same name
	c2 := api.Container{
		ID:   "reuse-new",
		Name: name,
		Config: api.ContainerConfig{
			Image:  "nginx:1.25",
			Labels: make(map[string]string),
		},
		State: api.ContainerState{Status: "running", Running: true},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	s.Store.Containers.Put(c2.ID, c2)
	s.Store.ContainerNames.Put(name, c2.ID)

	// Verify new container has the name
	id, ok := s.Store.ContainerNames.Get(name)
	if !ok || id != "reuse-new" {
		t.Errorf("expected name to map to reuse-new, got %q", id)
	}

	// Verify old container is gone
	if _, ok := s.Store.Containers.Get("reuse-old"); ok {
		t.Error("expected old container to be gone")
	}

	// Verify new container exists
	got, ok := s.Store.Containers.Get("reuse-new")
	if !ok {
		t.Fatal("expected new container to exist")
	}
	if got.Config.Image != "nginx:1.25" {
		t.Errorf("new container image = %q, want nginx:1.25", got.Config.Image)
	}
}
