package lambda

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

func TestParseSharedVolumes(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		for _, in := range []string{"", "   ", " , "} {
			vols, err := parseSharedVolumes(in)
			if err != nil {
				t.Fatalf("parseSharedVolumes(%q) error: %v", in, err)
			}
			if len(vols) != 0 {
				t.Fatalf("parseSharedVolumes(%q) = %v, want empty", in, vols)
			}
		}
	})

	t.Run("valid 3- to 5-tuples", func(t *testing.T) {
		vols, err := parseSharedVolumes("ws=/home/runner/_work=fsap-1, ext=/home/runner/externals=fsap-1==externals, tool=/opt/tool=fsap-1=fs-9=tooldir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vols) != 3 {
			t.Fatalf("got %d volumes, want 3", len(vols))
		}
		if vols[0].Name != "ws" || vols[0].AccessPointID != "fsap-1" || vols[0].EFSSubpath != "" {
			t.Errorf("vols[0] = %+v", vols[0])
		}
		if vols[1].FileSystemID != "" || vols[1].EFSSubpath != "externals" {
			t.Errorf("vols[1] = %+v", vols[1])
		}
		if vols[2].FileSystemID != "fs-9" || vols[2].EFSSubpath != "tooldir" {
			t.Errorf("vols[2] = %+v", vols[2])
		}
	})

	t.Run("malformed entries error", func(t *testing.T) {
		for _, in := range []string{
			"ws=/home/runner/_work",           // too few fields
			"ws=/p=fsap-1=fs-1=subpath=extra", // too many fields
			"=/home/runner/_work=fsap-1",      // empty name
			"ws=/home/runner/_work=",          // empty access point
			"ok=/a=fsap-1,ws=/home/runner",    // one valid + one malformed
		} {
			if _, err := parseSharedVolumes(in); err == nil {
				t.Errorf("parseSharedVolumes(%q) = nil error, want parse error", in)
			}
		}
	})
}

func TestValidateSurfacesSharedVolumesParseError(t *testing.T) {
	cfg := Config{
		RoleARN:          "arn:aws:iam::000000000000:role/x",
		Architecture:     "arm64",
		NetworkDiscovery: api.NetworkDiscoveryNATGatewayOnly,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
	cfg.sharedVolumesErr = errors.New("entry \"junk\" malformed")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want shared-volumes parse error")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_LAMBDA_SHARED_VOLUMES") {
		t.Errorf("error %q does not mention SOCKERLESS_LAMBDA_SHARED_VOLUMES", err)
	}
}

// newSharedVolumeTestServer builds a Server whose EFS client talks to a
// stub elasticfilesystem REST endpoint serving the single access point
// fsap-1 / fs-1. Only DescribeAccessPoints is needed by the
// shared-volume bind path (the AP itself is operator-provisioned).
func newSharedVolumeTestServer(t *testing.T) *Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/2015-02-01/access-points" {
			apID := r.URL.Query().Get("AccessPointId")
			if apID == "" {
				apID = "fsap-1"
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"AccessPoints":[{"AccessPointId":%q,"AccessPointArn":"arn:aws:elasticfilesystem:us-east-1:000000000000:access-point/%s","FileSystemId":"fs-1","LifeCycleState":"available"}]}`, apID, apID)
			return
		}
		t.Errorf("unexpected EFS API call: %s %s", r.Method, r.URL)
		http.Error(w, "unexpected call", http.StatusBadRequest)
	}))
	t.Cleanup(ts.Close)

	client := efs.New(efs.Options{
		Region:           "us-east-1",
		BaseEndpoint:     aws.String(ts.URL),
		Credentials:      aws.AnonymousCredentials{},
		RetryMaxAttempts: 1,
	})
	s := &Server{
		config: Config{
			SubnetIDs:  []string{"subnet-1"},
			AgentEFSID: "fs-1",
			SharedVolumes: []SharedVolume{
				{Name: "ws", ContainerPath: "/home/runner/_work", AccessPointID: "fsap-1"},
			},
		},
	}
	s.volumeState = volumeState{efs: awscommon.NewEFSManager(client, awscommon.EFSManagerConfig{AgentEFSID: "fs-1"})}
	s.storageBackings = core.NewStorageBackingRegistry()
	s.storageBackings.Register(awscommon.NewEFSEphemeralDriver(s.efs))
	return s
}

func TestFileSystemConfigsForBinds(t *testing.T) {
	ctx := context.Background()

	t.Run("mapped bind translates to single FileSystemConfig + bind link", func(t *testing.T) {
		s := newSharedVolumeTestServer(t)
		fscs, links, err := s.fileSystemConfigsForBinds(ctx, []string{
			"/home/runner/_work:/__w",
			"/home/runner/_work/_temp:/__w/_temp", // sub-path → dropped
			"/var/run/docker.sock:/var/run/docker.sock",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fscs) != 1 {
			t.Fatalf("got %d FileSystemConfigs, want 1", len(fscs))
		}
		wantARN := "arn:aws:elasticfilesystem:us-east-1:000000000000:access-point/fsap-1"
		if aws.ToString(fscs[0].Arn) != wantARN {
			t.Errorf("Arn = %q, want %q", aws.ToString(fscs[0].Arn), wantARN)
		}
		if aws.ToString(fscs[0].LocalMountPath) != LambdaSharedMountPath {
			t.Errorf("LocalMountPath = %q, want %q", aws.ToString(fscs[0].LocalMountPath), LambdaSharedMountPath)
		}
		if len(links) != 1 || links[0] != "/__w="+LambdaSharedMountPath {
			t.Errorf("links = %v, want [/__w=%s]", links, LambdaSharedMountPath)
		}
	})

	t.Run("drop-only binds yield no FileSystemConfig", func(t *testing.T) {
		s := newSharedVolumeTestServer(t)
		fscs, links, err := s.fileSystemConfigsForBinds(ctx, []string{
			"/home/runner/_work/_temp:/__w/_temp",
			"/var/run/docker.sock:/var/run/docker.sock",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fscs != nil || links != nil {
			t.Errorf("fscs=%v links=%v, want nil/nil", fscs, links)
		}
	})

	t.Run("unmapped host bind rejects with configure hint", func(t *testing.T) {
		s := newSharedVolumeTestServer(t)
		_, _, err := s.fileSystemConfigsForBinds(ctx, []string{"/not/mapped:/x"})
		if err == nil {
			t.Fatal("want rejection for unmapped host bind")
		}
		if !strings.Contains(err.Error(), "SOCKERLESS_LAMBDA_SHARED_VOLUMES") {
			t.Errorf("error %q does not mention SOCKERLESS_LAMBDA_SHARED_VOLUMES", err)
		}
	})

	t.Run("binds without VPC subnets reject", func(t *testing.T) {
		s := newSharedVolumeTestServer(t)
		s.config.SubnetIDs = nil
		_, _, err := s.fileSystemConfigsForBinds(ctx, []string{"ws:/__w"})
		if err == nil || !strings.Contains(err.Error(), "SOCKERLESS_LAMBDA_SUBNETS") {
			t.Errorf("error %v does not mention SOCKERLESS_LAMBDA_SUBNETS", err)
		}
	})
}
