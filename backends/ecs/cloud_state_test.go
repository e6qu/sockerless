package ecs

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestMapTaskStatus_Running(t *testing.T) {
	startedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	task := ecstypes.Task{
		LastStatus: aws.String("RUNNING"),
		StartedAt:  &startedAt,
	}

	state := mapTaskStatus(task)

	if state.Status != "running" {
		t.Fatalf("expected status 'running', got %q", state.Status)
	}
	if !state.Running {
		t.Fatal("expected Running=true")
	}
	if state.StartedAt == "" {
		t.Fatal("expected StartedAt to be set")
	}
}

func TestMapTaskStatus_StoppedExitCode0(t *testing.T) {
	exitCode := int32(0)
	stoppedAt := time.Date(2025, 6, 1, 13, 0, 0, 0, time.UTC)
	task := ecstypes.Task{
		LastStatus:    aws.String("STOPPED"),
		StoppedAt:     &stoppedAt,
		StoppedReason: aws.String("Essential container in task exited"),
		Containers: []ecstypes.Container{
			{ExitCode: &exitCode},
		},
	}

	state := mapTaskStatus(task)

	if state.Status != "exited" {
		t.Fatalf("expected status 'exited', got %q", state.Status)
	}
	if state.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", state.ExitCode)
	}
}

func TestMapTaskStatus_StoppedExitCode1(t *testing.T) {
	exitCode := int32(1)
	task := ecstypes.Task{
		LastStatus: aws.String("STOPPED"),
		Containers: []ecstypes.Container{
			{ExitCode: &exitCode},
		},
	}

	state := mapTaskStatus(task)

	if state.Status != "exited" {
		t.Fatalf("expected status 'exited', got %q", state.Status)
	}
	if state.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", state.ExitCode)
	}
}

func TestMapTaskStatus_Pending(t *testing.T) {
	task := ecstypes.Task{
		LastStatus: aws.String("PENDING"),
	}

	state := mapTaskStatus(task)

	if state.Status != "created" {
		t.Fatalf("expected status 'created', got %q", state.Status)
	}
}

func TestMapTaskStatus_Provisioning(t *testing.T) {
	task := ecstypes.Task{
		LastStatus: aws.String("PROVISIONING"),
	}

	state := mapTaskStatus(task)

	if state.Status != "created" {
		t.Fatalf("expected status 'created', got %q", state.Status)
	}
}

func TestTaskToContainer_BasicFields(t *testing.T) {
	createdAt := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	task := ecstypes.Task{
		LastStatus: aws.String("RUNNING"),
		StartedAt:  &createdAt,
		CreatedAt:  &createdAt,
		Containers: []ecstypes.Container{
			{
				Name:  aws.String("main"),
				Image: aws.String("nginx:latest"),
			},
		},
	}
	tags := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		"sockerless-name":         "/my-nginx",
	}

	c := taskToContainer(task, tags)

	if c.ID != "abc123def456abc123def456abc123def456abc123def456abc123def456abcd" {
		t.Fatalf("expected container ID from tag, got %q", c.ID)
	}
	if c.Name != "/my-nginx" {
		t.Fatalf("expected name '/my-nginx', got %q", c.Name)
	}
	if c.Image != "nginx:latest" {
		t.Fatalf("expected image 'nginx:latest', got %q", c.Image)
	}
	if c.State.Status != "running" {
		t.Fatalf("expected status 'running', got %q", c.State.Status)
	}
}

func TestTaskToContainer_LabelsFromTags(t *testing.T) {
	task := ecstypes.Task{
		LastStatus: aws.String("RUNNING"),
		Containers: []ecstypes.Container{
			{Name: aws.String("main"), Image: aws.String("alpine")},
		},
	}
	tags := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		"sockerless-name":         "/test",
		"sockerless-labels":       `{"app":"web","env":"prod"}`,
	}

	c := taskToContainer(task, tags)

	if c.Config.Labels["app"] != "web" {
		t.Fatalf("expected label app=web, got %q", c.Config.Labels["app"])
	}
	if c.Config.Labels["env"] != "prod" {
		t.Fatalf("expected label env=prod, got %q", c.Config.Labels["env"])
	}
}

func TestTaskToContainer_NetworkIPFromENI(t *testing.T) {
	task := ecstypes.Task{
		LastStatus: aws.String("RUNNING"),
		Containers: []ecstypes.Container{
			{Name: aws.String("main"), Image: aws.String("alpine")},
		},
		Attachments: []ecstypes.Attachment{
			{
				Type: aws.String("ElasticNetworkInterface"),
				Details: []ecstypes.KeyValuePair{
					{Name: aws.String("privateIPv4Address"), Value: aws.String("10.0.1.42")},
				},
			},
		},
	}
	tags := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		"sockerless-name":         "/test",
	}

	c := taskToContainer(task, tags)

	net := c.NetworkSettings.Networks["bridge"]
	if net == nil {
		t.Fatal("expected bridge network")
	}
	if net.IPAddress != "10.0.1.42" {
		t.Fatalf("expected IP 10.0.1.42, got %q", net.IPAddress)
	}
	if net.MacAddress != "02:42:0a:00:01:2a" {
		t.Fatalf("expected MAC 02:42:0a:00:01:2a, got %q", net.MacAddress)
	}
}

func TestTaskToContainer_DefaultNameFromID(t *testing.T) {
	task := ecstypes.Task{
		LastStatus: aws.String("PENDING"),
		Containers: []ecstypes.Container{
			{Name: aws.String("main"), Image: aws.String("alpine")},
		},
	}
	tags := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
	}

	c := taskToContainer(task, tags)

	if c.Name != "/abc123def456" {
		t.Fatalf("expected default name '/abc123def456', got %q", c.Name)
	}
}
