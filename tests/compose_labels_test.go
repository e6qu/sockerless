package tests

import (
	"testing"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
)

func TestComposeContainerLabelFilter(t *testing.T) {
	// Create containers with compose-style labels
	id1 := createContainer(t, "compose-svc-1", &container.Config{
		Image: "alpine:latest",
		Labels: map[string]string{
			"com.docker.compose.project": "myapp",
			"com.docker.compose.service": "web",
		},
	}, nil)
	defer removeContainer(t, id1)

	id2 := createContainer(t, "compose-svc-2", &container.Config{
		Image: "alpine:latest",
		Labels: map[string]string{
			"com.docker.compose.project": "myapp",
			"com.docker.compose.service": "db",
		},
	}, nil)
	defer removeContainer(t, id2)

	id3 := createContainer(t, "compose-svc-3", &container.Config{
		Image: "alpine:latest",
		Labels: map[string]string{
			"com.docker.compose.project": "other",
			"com.docker.compose.service": "api",
		},
	}, nil)
	defer removeContainer(t, id3)

	// Filter by project label
	f := client.Filters{}
	f.Add("label", "com.docker.compose.project=myapp")
	listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	containers := listed.Items
	if err != nil {
		t.Fatalf("container list failed: %v", err)
	}

	if len(containers) != 2 {
		t.Errorf("expected 2 containers with project=myapp, got %d", len(containers))
	}

	for _, c := range containers {
		if c.Labels["com.docker.compose.project"] != "myapp" {
			t.Errorf("unexpected project label: %s", c.Labels["com.docker.compose.project"])
		}
	}
}

func TestComposeKeyOnlyLabelFilter(t *testing.T) {
	id1 := createContainer(t, "labeled-1", &container.Config{
		Image: "alpine:latest",
		Labels: map[string]string{
			"com.docker.compose.project": "test",
		},
	}, nil)
	defer removeContainer(t, id1)

	id2 := createContainer(t, "unlabeled-1", &container.Config{
		Image: "alpine:latest",
	}, nil)
	defer removeContainer(t, id2)

	// Filter by key-only label (just check existence)
	f := client.Filters{}
	f.Add("label", "com.docker.compose.project")
	listed2, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	containers := listed2.Items
	if err != nil {
		t.Fatalf("container list failed: %v", err)
	}

	if len(containers) != 1 {
		t.Errorf("expected 1 container with compose.project label, got %d", len(containers))
	}
}

func TestComposeNetworkLabelFilter(t *testing.T) {
	// Create networks with labels
	resp1, err := dockerClient.NetworkCreate(ctx, "compose-net-1", client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"com.docker.compose.project": "myapp",
			"com.docker.compose.network": "default",
		},
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer removeNetwork(t, resp1.ID)

	resp2, err := dockerClient.NetworkCreate(ctx, "compose-net-2", client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"com.docker.compose.project": "other",
		},
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer removeNetwork(t, resp2.ID)

	// Filter by label
	f := client.Filters{}
	f.Add("label", "com.docker.compose.project=myapp")
	netListed, err := dockerClient.NetworkList(ctx, client.NetworkListOptions{
		Filters: f,
	})
	networks := netListed.Items
	if err != nil {
		t.Fatalf("network list failed: %v", err)
	}

	found := false
	for _, n := range networks {
		if n.ID == resp1.ID {
			found = true
		}
		if n.ID == resp2.ID {
			t.Error("unexpected network from other project in filtered results")
		}
	}
	if !found {
		t.Error("expected compose-net-1 in filtered results")
	}
}

func TestComposeNetworkPruneWithLabels(t *testing.T) {
	// Create a network with compose labels
	resp, err := dockerClient.NetworkCreate(ctx, "compose-prune-net", client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"com.docker.compose.project": "prune-test",
		},
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}

	// Prune with label filter
	f := client.Filters{}
	f.Add("label", "com.docker.compose.project=prune-test")
	pruned, err := dockerClient.NetworkPrune(ctx, client.NetworkPruneOptions{Filters: f})
	report := pruned.Report
	if err != nil {
		t.Fatalf("network prune failed: %v", err)
	}

	found := false
	for _, id := range report.NetworksDeleted {
		if id == resp.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected pruned network in deleted list")
	}
}

func TestComposeLifecycle(t *testing.T) {
	projectName := "compose-lifecycle"

	// Create network
	netResp, err := dockerClient.NetworkCreate(ctx, projectName+"_default", client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.network": "default",
		},
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer removeNetwork(t, netResp.ID)

	// Create containers
	id1 := createContainer(t, projectName+"-web-1", &container.Config{
		Image: "alpine:latest",
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.service": "web",
		},
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode(projectName + "_default"),
	})
	defer removeContainer(t, id1)

	id2 := createContainer(t, projectName+"-db-1", &container.Config{
		Image: "alpine:latest",
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.service": "db",
		},
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode(projectName + "_default"),
	})
	defer removeContainer(t, id2)

	// List by project
	f := client.Filters{}
	f.Add("label", "com.docker.compose.project="+projectName)
	listed3, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	containers := listed3.Items
	if err != nil {
		t.Fatalf("container list failed: %v", err)
	}

	if len(containers) != 2 {
		t.Errorf("expected 2 containers in project, got %d", len(containers))
	}

	// List networks by project
	nf := client.Filters{}
	nf.Add("label", "com.docker.compose.project="+projectName)
	netListed2, err := dockerClient.NetworkList(ctx, client.NetworkListOptions{
		Filters: nf,
	})
	networks := netListed2.Items
	if err != nil {
		t.Fatalf("network list failed: %v", err)
	}

	foundNet := false
	for _, n := range networks {
		if n.ID == netResp.ID {
			foundNet = true
		}
	}
	if !foundNet {
		t.Error("expected project network in filtered results")
	}
}
