package ecs

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/sockerless/api"
)

// testServer returns a minimal Server with config fields needed by buildContainerDef.
func testServer() *Server {
	return &Server{
		config: Config{
			LogGroup: "/sockerless",
			Region:   "us-east-1",
		},
	}
}

func testInput(c *api.Container) containerInput {
	return containerInput{
		ID:        "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Container: c,
		IsMain:    true,
	}
}

func TestBuildContainerDef_BasicImage(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config:     api.ContainerConfig{Image: "nginx:latest"},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if aws.ToString(def.Image) != "nginx:latest" {
		t.Fatalf("expected image nginx:latest, got %s", aws.ToString(def.Image))
	}
	if aws.ToString(def.Name) != "main" {
		t.Fatalf("expected container name 'main', got %s", aws.ToString(def.Name))
	}
	if !aws.ToBool(def.Essential) {
		t.Fatal("expected essential=true for main container")
	}
}

func TestShellQuoteArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"single plain", []string{"echo"}, "'echo'"},
		{"two plain", []string{"echo", "hello"}, "'echo' 'hello'"},
		{"with space", []string{"echo", "hello world"}, "'echo' 'hello world'"},
		{"with single quote", []string{"echo", "it's"}, "'echo' 'it'\\''s'"},
		{"with $", []string{"sh", "-c", "echo $HOME"}, "'sh' '-c' 'echo $HOME'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellQuoteArgs(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestBuildContainerDef_Entrypoint(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config: api.ContainerConfig{
			Image:      "alpine",
			Entrypoint: []string{"/bin/sh", "-c"},
		},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if len(def.EntryPoint) != 2 || def.EntryPoint[0] != "/bin/sh" || def.EntryPoint[1] != "-c" {
		t.Fatalf("expected entrypoint [/bin/sh -c], got %v", def.EntryPoint)
	}
}

func TestBuildContainerDef_Command(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config: api.ContainerConfig{
			Image: "alpine",
			Cmd:   []string{"echo", "hello"},
		},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if len(def.Command) != 2 || def.Command[0] != "echo" || def.Command[1] != "hello" {
		t.Fatalf("expected command [echo hello], got %v", def.Command)
	}
}

func TestBuildContainerDef_EnvVars(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config: api.ContainerConfig{
			Image: "alpine",
			Env:   []string{"FOO=bar", "BAZ=qux"},
		},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if len(def.Environment) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(def.Environment))
	}
	if aws.ToString(def.Environment[0].Name) != "FOO" || aws.ToString(def.Environment[0].Value) != "bar" {
		t.Fatalf("expected FOO=bar, got %s=%s", aws.ToString(def.Environment[0].Name), aws.ToString(def.Environment[0].Value))
	}
	if aws.ToString(def.Environment[1].Name) != "BAZ" || aws.ToString(def.Environment[1].Value) != "qux" {
		t.Fatalf("expected BAZ=qux, got %s=%s", aws.ToString(def.Environment[1].Name), aws.ToString(def.Environment[1].Value))
	}
}

func TestBuildContainerDef_WorkingDir(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config: api.ContainerConfig{
			Image:      "alpine",
			WorkingDir: "/app",
		},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if aws.ToString(def.WorkingDirectory) != "/app" {
		t.Fatalf("expected working dir /app, got %s", aws.ToString(def.WorkingDirectory))
	}
}

func TestBuildContainerDef_User(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config: api.ContainerConfig{
			Image: "alpine",
			User:  "nobody",
		},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if aws.ToString(def.User) != "nobody" {
		t.Fatalf("expected user 'nobody', got %s", aws.ToString(def.User))
	}
}

func TestBuildContainerDef_TTY(t *testing.T) {
	s := testServer()
	ci := testInput(&api.Container{
		Config: api.ContainerConfig{
			Image: "alpine",
			Tty:   true,
		},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if !aws.ToBool(def.PseudoTerminal) {
		t.Fatal("expected PseudoTerminal=true when Tty=true")
	}
}

// Named-volume-to-EFS-access-point resolution lives in volumes.go and
// requires a live EFS client; it's covered by integration tests
// against the AWS simulator (`backends/ecs/integration_test.go`).
// buildContainerDef's no-bind path is exercised by the other
// TestBuildContainerDef_* unit tests above.

func TestBuildContainerDef_LogConfig(t *testing.T) {
	s := testServer()
	s.config.LogGroup = "/my-log-group"
	s.config.Region = "eu-west-1"

	ci := testInput(&api.Container{
		Config:     api.ContainerConfig{Image: "alpine"},
		HostConfig: api.HostConfig{},
	})

	def, _, _ := s.buildContainerDef(context.Background(), ci)

	if def.LogConfiguration == nil {
		t.Fatal("expected log configuration to be set")
	}
	if def.LogConfiguration.LogDriver != "awslogs" {
		t.Fatalf("expected awslogs driver, got %s", def.LogConfiguration.LogDriver)
	}
	if def.LogConfiguration.Options["awslogs-group"] != "/my-log-group" {
		t.Fatalf("expected log group /my-log-group, got %s", def.LogConfiguration.Options["awslogs-group"])
	}
	if def.LogConfiguration.Options["awslogs-region"] != "eu-west-1" {
		t.Fatalf("expected region eu-west-1, got %s", def.LogConfiguration.Options["awslogs-region"])
	}
	// Stream prefix is first 12 chars of container ID
	if def.LogConfiguration.Options["awslogs-stream-prefix"] != "abcdef123456" {
		t.Fatalf("expected stream prefix abcdef123456, got %s", def.LogConfiguration.Options["awslogs-stream-prefix"])
	}
}

func TestFargateResources_DefaultMinimum(t *testing.T) {
	containers := []containerInput{{
		Container: &api.Container{HostConfig: api.HostConfig{}},
	}}
	cpu, mem := fargateResources(containers)
	if cpu != "256" || mem != "512" {
		t.Fatalf("expected 256/512 for no constraints, got %s/%s", cpu, mem)
	}
}

func TestFargateResources_MemoryMapping(t *testing.T) {
	// Request 3GB memory -> fits 512 CPU tier (supports up to 4096 MB)
	containers := []containerInput{{
		Container: &api.Container{
			HostConfig: api.HostConfig{
				Memory: 3 * 1024 * 1024 * 1024, // 3GB in bytes
			},
		},
	}}
	cpu, mem := fargateResources(containers)
	if cpu != "512" || mem != "3072" {
		t.Fatalf("expected 512/3072, got %s/%s", cpu, mem)
	}
}

func TestFargateResources_CPUMapping(t *testing.T) {
	// Request 512 CPU shares, no memory -> smallest mem for that CPU tier
	containers := []containerInput{{
		Container: &api.Container{
			HostConfig: api.HostConfig{
				CPUShares: 512,
			},
		},
	}}
	cpu, mem := fargateResources(containers)
	if cpu != "512" || mem != "1024" {
		t.Fatalf("expected 512/1024, got %s/%s", cpu, mem)
	}
}

func TestFargateResources_NanoCPUs(t *testing.T) {
	// 2 vCPU = 2e9 NanoCPUs -> 2048 CPU units
	containers := []containerInput{{
		Container: &api.Container{
			HostConfig: api.HostConfig{
				NanoCPUs: 2e9,
			},
		},
	}}
	cpu, mem := fargateResources(containers)
	if cpu != "2048" {
		t.Fatalf("expected cpu 2048, got %s", cpu)
	}
	if mem != "4096" {
		t.Fatalf("expected mem 4096, got %s", mem)
	}
}

func TestSanitizeContainerName(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"/my-container", "my-container"},
		{"my.container.name", "my-container-name"},
		{"", "sidecar"},
		{"/", "sidecar"},
		{"valid_name-123", "valid_name-123"},
	}
	for _, tt := range tests {
		got := sanitizeContainerName(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeContainerName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
