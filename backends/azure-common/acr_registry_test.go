package azurecommon

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// End-to-end coverage of the Azure Container Registry credential path against
// a real Azure Container Registry: an image is pushed, tagged, listed and
// removed over the Docker Registry HTTP API v2, with every request carrying a
// credential the registry's own token service issued.
//
// The registry is the Microsoft Azure simulator, which is a real
// implementation of the Azure APIs rather than a mock, and the code under test
// is the code that runs against Azure — the only difference is coordinates:
// SOCKERLESS_AZURE_ACR_ENDPOINT names the address the registry is reached at,
// IDENTITY_ENDPOINT/IDENTITY_HEADER name the managed identity endpoint that
// issues the Microsoft Entra token, and the Azure Resource Manager client is
// pointed at the same address. Everything else — the login server in the image
// reference, the exchange at /oauth2/exchange, the scoped token at
// /oauth2/token, the Bearer on /v2/ — is what runs against real Azure.

const (
	// acrSimSubscription and acrSimResourceGroup are the Azure Resource
	// Manager identifiers the registry is provisioned under. They are fixtures
	// of this test, the way an operator's subscription and resource group are
	// fixtures of their environment.
	acrSimSubscription  = "00000000-0000-0000-0000-000000000001"
	acrSimResourceGroup = "acr-auth-rg"
	acrSimRegistryName  = "sockerlessacr"

	// acrSimIdentityHeader is the shared secret the Azure platform injects as
	// IDENTITY_HEADER alongside IDENTITY_ENDPOINT. DefaultAzureCredential
	// echoes it back when it acquires a managed-identity token.
	acrSimIdentityHeader = "sim-identity-header"

	// acrTestRepository is the repository the round trip writes to.
	acrTestRepository = "sockerless/acr-auth"
)

var (
	// acrSimURL is the address the Azure simulator serves on: the Azure
	// Resource Manager plane, the managed identity endpoint, the registry's
	// token service and its /v2/ data plane all answer here.
	acrSimURL string
	// acrLoginServer is the registry's advertised login server — the host every
	// image reference names and every request's Host header carries.
	acrLoginServer string
	// acrEntraCredential is the Microsoft Entra identity the backend
	// authenticates as, resolved exactly as the backend resolves it.
	acrEntraCredential azcore.TokenCredential
)

func TestMain(m *testing.M) {
	code, cleanup := runWithAzureSimulator(m)
	cleanup()
	os.Exit(code)
}

// runWithAzureSimulator brings up the Microsoft Azure simulator, provisions a
// container registry in it, and points the ambient Azure coordinates at it.
func runWithAzureSimulator(m *testing.M) (int, func()) {
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	repoRoot := findRepoRoot()

	// The simulator lives in the sockerless-cloud repository and the tests
	// module pins its version, so it is built through that module — one source
	// of truth for which simulator this repository is tested against.
	simBinary := repoRoot + "/tests/.build/simulator-azure"
	if err := os.MkdirAll(repoRoot+"/tests/.build", 0o755); err != nil {
		log.Fatalf("create tests/.build: %v", err)
	}
	build := exec.Command("go", "build", "-tags", "noui", "-o", simBinary,
		"github.com/e6qu/sockerless-cloud/simulator-azure")
	build.Dir = repoRoot + "/tests"
	build.Env = withoutCrossCompileEnv(os.Environ())
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		log.Fatalf("build simulator-azure: %v", err)
	}

	port := freePort()
	acrSimURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	sim := exec.Command(simBinary)
	sim.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
		// The registry's advertised login server. An Azure deployment
		// advertises `<name>.azurecr.io`; this is the same coordinate,
		// configured on the simulator so it advertises the host its clients
		// name — and reach through SOCKERLESS_AZURE_ACR_ENDPOINT.
		`SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON={"acr":"https://{name}.azurecr.io/"}`,
	)
	sim.Stdout = os.Stderr
	sim.Stderr = os.Stderr
	if err := sim.Start(); err != nil {
		log.Fatalf("start simulator-azure: %v", err)
	}
	cleanups = append(cleanups, func() {
		_ = sim.Process.Kill()
		_ = sim.Wait()
	})
	if err := waitForHealth(acrSimURL+"/health", 30*time.Second); err != nil {
		log.Fatalf("simulator-azure never became ready: %v", err)
	}

	// The managed-identity coordinate the Azure platform injects into a
	// Container Apps / App Service container. DefaultAzureCredential performs a
	// real managed-identity token acquisition against it.
	setEnvForRun(&cleanups, "IDENTITY_ENDPOINT", acrSimURL+"/msi/token")
	setEnvForRun(&cleanups, "IDENTITY_HEADER", acrSimIdentityHeader)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Fatalf("Microsoft Entra credential: %v", err)
	}
	acrEntraCredential = cred

	acrLoginServer, err = provisionRegistry(cred)
	if err != nil {
		log.Fatalf("provision container registry: %v", err)
	}

	return m.Run(), cleanup
}

// provisionRegistry creates the container registry through the Azure Resource
// Manager, the way an operator provisions one, and returns the login server it
// advertises.
func provisionRegistry(cred azcore.TokenCredential) (string, error) {
	opts := &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
		Cloud: cloud.Configuration{Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
			cloud.ResourceManager: {Endpoint: acrSimURL, Audience: "https://management.azure.com/"},
		}},
		InsecureAllowCredentialWithHTTP: true,
	}}
	client, err := armcontainerregistry.NewRegistriesClient(acrSimSubscription, cred, opts)
	if err != nil {
		return "", fmt.Errorf("registries client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	poller, err := client.BeginCreate(ctx, acrSimResourceGroup, acrSimRegistryName, armcontainerregistry.Registry{
		Location: to.Ptr("eastus"),
		SKU:      &armcontainerregistry.SKU{Name: to.Ptr(armcontainerregistry.SKUNameStandard)},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("create registry: %w", err)
	}
	created, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("await registry: %w", err)
	}
	if created.Properties == nil || created.Properties.LoginServer == nil || *created.Properties.LoginServer == "" {
		return "", fmt.Errorf("registry advertises no login server")
	}
	return *created.Properties.LoginServer, nil
}

// newSimACRAuthProvider builds the provider with the registry endpoint
// coordinate an operator sets in SOCKERLESS_AZURE_ACR_ENDPOINT, passed
// explicitly so it applies to this harness alone. The reference still names the
// login server; the coordinate is only the address it is reached at.
func newSimACRAuthProvider() *ACRAuthProvider {
	return NewACRAuthProvider(zerolog.Nop(), acrSimURL)
}

// TestACRAuthProviderMintsRegistryAccessToken proves the credential the
// provider hands to the registry is one the registry's own token service
// issued, and not the Microsoft Entra token that was exchanged for it. An
// Azure Container Registry refuses a raw Entra token on /v2/, so presenting
// one is the defect this asserts against.
func TestACRAuthProviderMintsRegistryAccessToken(t *testing.T) {
	provider := newSimACRAuthProvider()

	token, err := provider.GetToken(acrLoginServer, acrTestRepository, core.ActionPull, core.ActionPush)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	registryToken, ok := strings.CutPrefix(token, "Bearer ")
	if !ok {
		t.Fatalf("GetToken returned %q, which carries no Bearer scheme", token)
	}
	if registryToken == "" {
		t.Fatal("GetToken returned an empty credential")
	}

	entra, err := acrEntraCredential.GetToken(context.Background(),
		policy.TokenRequestOptions{Scopes: []string{acrEntraScope}})
	if err != nil {
		t.Fatalf("Microsoft Entra token: %v", err)
	}
	if registryToken == entra.Token {
		t.Fatal("GetToken handed back the Microsoft Entra token itself; " +
			"an Azure Container Registry only accepts a token its own /oauth2 token service issued")
	}

	// The registry accepts it on the data plane.
	req, err := http.NewRequest(http.MethodGet, acrSimURL+"/v2/", nil)
	if err != nil {
		t.Fatalf("build /v2/ request: %v", err)
	}
	core.SetOCIHost(req, acrLoginServer)
	core.SetOCIAuth(req, token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v2/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the registry refused the token its token service issued: GET /v2/ returned %d", resp.StatusCode)
	}
}

// TestACRAuthProviderScopesTokenToTheOperation proves the token is asked for
// with the access the operation needs, in the Docker Registry HTTP API v2
// scope grammar the registry's own challenge names.
func TestACRAuthProviderScopesTokenToTheOperation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		repository string
		actions    []string
		want       string
	}{
		{"pull a repository", "team/app", []string{core.ActionPull}, "repository:team/app:pull"},
		{"write a repository", "team/app", []string{core.ActionPull, core.ActionPush}, "repository:team/app:pull,push"},
		{"remove from a repository", "team/app", []string{core.ActionPull, core.ActionDelete}, "repository:team/app:pull,delete"},
		{"read a repository's tags", "team/app", []string{core.ActionMetadataRead}, "repository:team/app:metadata_read"},
		{"the registry's catalog", "", nil, "registry:catalog:*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ACRRegistryScope(tc.repository, tc.actions...); got != tc.want {
				t.Fatalf("scope = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestACRRegistryRoundTrip pushes a real image to the registry, adds a second
// tag to it, lists what the registry holds, and removes it again — every
// request authenticated by the registry's token service and routed through the
// endpoint coordinate.
func TestACRRegistryRoundTrip(t *testing.T) {
	provider := newSimACRAuthProvider()
	ref := acrLoginServer + "/" + acrTestRepository

	if !provider.IsCloudRegistry(acrLoginServer) {
		t.Fatalf("%s must be recognized as an Azure Container Registry", acrLoginServer)
	}
	if got := provider.RegistryEndpoint(acrLoginServer); got != acrSimURL {
		t.Fatalf("RegistryEndpoint = %q, want the configured coordinate %q", got, acrSimURL)
	}

	pushToken, err := provider.GetToken(acrLoginServer, acrTestRepository, core.ActionPull, core.ActionPush)
	if err != nil {
		t.Fatalf("GetToken for push: %v", err)
	}

	layer := gzippedLayer(t, "sockerless.txt", "azure container registry round trip")
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layer))
	result, err := core.OCIPush(core.OCIPushOptions{
		Registry:   acrLoginServer,
		Endpoint:   provider.RegistryEndpoint(acrLoginServer),
		Repository: acrTestRepository,
		Tag:        "v1",
		AuthToken:  pushToken,
		ImageLayers: []string{
			fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("sockerless.txt"))),
		},
		ManifestLayers: []core.ManifestLayerEntry{{
			Digest:    layerDigest,
			Size:      int64(len(layer)),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		}},
		Architecture: "amd64",
		OS:           "linux",
		LayerContent: func(digest string) ([]byte, bool) {
			if digest == layerDigest {
				return layer, true
			}
			return nil, false
		},
	})
	if err != nil {
		t.Fatalf("push %s:v1: %v", ref, err)
	}
	if result.ManifestDigest == "" {
		t.Fatal("push returned no manifest digest")
	}

	// A second tag on the same manifest, through the provider's own sync path.
	if err := provider.OnTag(result.ManifestDigest, acrLoginServer, acrTestRepository, "v2"); err != nil {
		t.Fatalf("OnTag v2: %v", err)
	}

	tags := listRepositoryTags(t, provider, acrLoginServer, acrTestRepository)
	for _, want := range []string{ref + ":v1", ref + ":v2"} {
		if !tags[want] {
			t.Fatalf("registry listing is missing %q; it holds %v", want, keysOf(tags))
		}
	}

	if err := provider.OnRemove(acrLoginServer, acrTestRepository, []string{"v1", "v2"}); err != nil {
		t.Fatalf("OnRemove: %v", err)
	}
	remaining := listRepositoryTags(t, provider, acrLoginServer, acrTestRepository)
	for _, gone := range []string{ref + ":v1", ref + ":v2"} {
		if remaining[gone] {
			t.Fatalf("%q survived removal; the registry still holds %v", gone, keysOf(remaining))
		}
	}
}

// listRepositoryTags reads a repository's tags over GET /v2/<repo>/tags/list
// with a metadata_read-scoped token, which is the per-repository half of what
// the ACA and Azure Functions backends do to serve `docker images`. The
// registry-wide half — GET /v2/_catalog, which enumerates the repositories to
// read tags for — is not served by the Microsoft Azure simulator, so this
// exercises the repository listing directly rather than through
// core.OCIListImages.
func listRepositoryTags(t *testing.T, provider *ACRAuthProvider, registry, repository string) map[string]bool {
	t.Helper()
	token, err := provider.GetToken(registry, repository, core.ActionMetadataRead)
	if err != nil {
		t.Fatalf("GetToken for tag listing: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet,
		provider.RegistryEndpoint(registry)+"/v2/"+repository+"/tags/list", nil)
	if err != nil {
		t.Fatalf("build tags request: %v", err)
	}
	core.SetOCIHost(req, registry)
	core.SetOCIAuth(req, token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	defer resp.Body.Close()
	out := map[string]bool{}
	if resp.StatusCode == http.StatusNotFound {
		// The repository holds nothing, which is how a registry reports a
		// repository whose last manifest was removed.
		return out
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list tags returned %d", resp.StatusCode)
	}
	var page struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	for _, tag := range page.Tags {
		out[registry+"/"+repository+":"+tag] = true
	}
	return out
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// gzippedLayer builds a real image layer: a gzip-compressed tar holding one
// file, which is what a registry stores and verifies by digest.
func gzippedLayer(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// --- harness helpers ---

func setEnvForRun(cleanups *[]func(), key, value string) {
	previous, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		log.Fatalf("set %s: %v", key, err)
	}
	*cleanups = append(*cleanups, func() {
		if had {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func findRepoRoot() string {
	for _, candidate := range []string{"../..", "../../..", "."} {
		if _, err := os.Stat(candidate + "/go.work"); err == nil {
			return candidate
		}
	}
	log.Fatal("could not locate the repository root (no go.work above the module)")
	return ""
}

func withoutCrossCompileEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "GOOS=") || strings.HasPrefix(entry, "GOARCH=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func freePort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("reserve a port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHealth(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s: %w", timeout, last)
}
