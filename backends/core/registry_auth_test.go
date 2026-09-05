package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func dockerAuthHeader(t *testing.T, ac map[string]string) string {
	t.Helper()
	raw, err := json.Marshal(ac)
	if err != nil {
		t.Fatal(err)
	}
	return base64.URLEncoding.EncodeToString(raw)
}

// TestRegistryAuthorizationFromDockerAuth: every shape of X-Registry-Auth a
// Docker client sends becomes the Authorization value the registry takes, an
// identity token is refused rather than dropped, and a minted registry
// credential passes through.
func TestRegistryAuthorizationFromDockerAuth(t *testing.T) {
	basicAlice := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	cases := []struct {
		name    string
		encoded string
		want    string
		wantErr string
	}{
		{"empty", "", "", ""},
		{"empty config", dockerAuthHeader(t, map[string]string{}), "", ""},
		{"server address only", dockerAuthHeader(t, map[string]string{"serveraddress": "https://index.docker.io/v1/"}), "", ""},
		{"username and password", dockerAuthHeader(t, map[string]string{"username": "alice", "password": "s3cret"}), basicAlice, ""},
		{"pre-joined auth", dockerAuthHeader(t, map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))}), basicAlice, ""},
		{"registry token", dockerAuthHeader(t, map[string]string{"registrytoken": "tok"}), "Bearer tok", ""},
		{"identity token", dockerAuthHeader(t, map[string]string{"identitytoken": "refresh"}), "", "identity token"},
		{"standard base64 without padding", strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(`{"username":"alice","password":"s3cret"}`)), "="), basicAlice, ""},
		{"minted bearer passes through", "Bearer minted", "Bearer minted", ""},
		{"minted basic passes through", "Basic bWludGVk", "Basic bWludGVk", ""},
		{"not base64", "not base64!", "", "not base64"},
		{"not JSON", base64.URLEncoding.EncodeToString([]byte("plain")), "", "not a Docker AuthConfig"},
	}
	for _, tc := range cases {
		got, err := RegistryAuthorizationFromDockerAuth(tc.encoded)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

// privateRegistry serves the Docker Registry HTTP API v2 for two private
// repositories over TLS: `library/tokenimg` behind a token service that
// issues a Bearer for Alice's Basic credential, and `library/basicimg` behind
// a Basic challenge that takes her credential on every request. Anonymous
// reads of either are refused the way the real registries refuse them.
func privateRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	const aliceBasic = "Basic YWxpY2U6czNjcmV0" // alice:s3cret
	configDigest := "sha256:cfg1111111111111111111111111111111111111111111111111111111111111"
	configJSON, _ := json.Marshal(map[string]any{
		"architecture": workloadArch(), "os": "linux",
		"config": map[string]any{"Cmd": []string{"/bin/private"}},
	})
	manifestJSON, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"digest":    configDigest,
			"size":      len(configJSON),
		},
	})
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != aliceBasic {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "alice-bearer"})
	})
	serve := func(w http.ResponseWriter, r *http.Request, want, challenge string) {
		if r.Header.Get("Authorization") != want {
			w.Header().Set("Www-Authenticate", challenge)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
			_, _ = w.Write(manifestJSON)
			return
		}
		_, _ = w.Write(configJSON)
	}
	mux.HandleFunc("/v2/library/tokenimg/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, "Bearer alice-bearer", fmt.Sprintf(`Bearer realm="%s/token",service="private",scope="repository:library/tokenimg:pull"`, srv.URL))
	})
	mux.HandleFunc("/v2/library/basicimg/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, aliceBasic, `Basic realm="private"`)
	})
	srv = httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	// The registry is reached at https://<host> like any registry; trust
	// this server's certificate for the duration of the test.
	saved := registryClient.Transport
	registryClient.Transport = srv.Client().Transport
	t.Cleanup(func() { registryClient.Transport = saved })
	return srv
}

// TestImagePullCarriesDockerCredential: the shared ImagePull reads a private
// image with the credential the client sent, through both a token service and
// a Basic-only registry, and an anonymous read of the same image fails.
func TestImagePullCarriesDockerCredential(t *testing.T) {
	srv := privateRegistry(t)
	host := strings.TrimPrefix(srv.URL, "https://")
	alice := dockerAuthHeader(t, map[string]string{"username": "alice", "password": "s3cret"})

	for _, repo := range []string{"tokenimg", "basicimg"} {
		ref := host + "/library/" + repo + ":v1"
		s := newTestBaseServer()
		if _, err := s.ImagePull(ref, ""); err == nil {
			t.Fatalf("%s: an anonymous pull of a private image succeeded", repo)
		}
		if _, ok := s.Store.ResolveImage(ref); ok {
			t.Fatalf("%s: a failed pull stored an image", repo)
		}
		out, err := s.ImagePull(ref, alice)
		if err != nil {
			t.Fatalf("%s: pull with the client's credential: %v", repo, err)
		}
		_ = out.Close()
		img, ok := s.Store.ResolveImage(ref)
		if !ok {
			t.Fatalf("%s: pulled image not stored", repo)
		}
		if len(img.Config.Cmd) != 1 || img.Config.Cmd[0] != "/bin/private" {
			t.Errorf("%s: config read from the registry = %v, want [/bin/private]", repo, img.Config.Cmd)
		}
	}

	// The ImageManager takes the same path for a registry that is not one of
	// the backend's cloud registries.
	ref := host + "/library/tokenimg:v2"
	m := &ImageManager{Base: newTestBaseServer()}
	if _, err := m.Pull(ref, ""); err == nil {
		t.Fatal("ImageManager: an anonymous pull of a private image succeeded")
	}
	out, err := m.Pull(ref, alice)
	if err != nil {
		t.Fatalf("ImageManager pull with the client's credential: %v", err)
	}
	_ = out.Close()

	// A credential that cannot be presented is reported as such.
	var invalid *api.InvalidParameterError
	_, err = m.Pull(host+"/library/tokenimg:v3", dockerAuthHeader(t, map[string]string{"identitytoken": "refresh"}))
	if !errors.As(err, &invalid) {
		t.Fatalf("identity token: err = %v, want InvalidParameterError", err)
	}
}
