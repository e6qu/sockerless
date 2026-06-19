package azf

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
)

func b64json(t *testing.T, v any) *string {
	t.Helper()
	raw, _ := json.Marshal(v)
	s := base64.StdEncoding.EncodeToString(raw)
	return &s
}

func nv(name string, val *string) *armappservice.NameValuePair {
	return &armappservice.NameValuePair{Name: &name, Value: val}
}

// TestAZFSpecFromProps verifies the docker Cmd/Entrypoint are recovered exactly
// from the SOCKERLESS_* app-settings and that Azure/sockerless plumbing settings
// are filtered out of the user Env.
func TestAZFSpecFromProps(t *testing.T) {
	user := "production"
	props := &armappservice.SiteProperties{
		SiteConfig: &armappservice.SiteConfig{
			AppSettings: []*armappservice.NameValuePair{
				nv("SOCKERLESS_ENTRYPOINT", b64json(t, []string{"/bin/sh", "-c"})),
				nv("SOCKERLESS_CMD", b64json(t, []string{"echo", "hi"})),
				nv("APP_ENV", &user),
				nv("FUNCTIONS_EXTENSION_VERSION", strPtr("~4")),
				nv("WEBSITES_ENABLE_APP_SERVICE_STORAGE", strPtr("false")),
				nv("DOCKER_REGISTRY_SERVER_URL", strPtr("x.azurecr.io")),
				nv("SOCKERLESS_CONTAINER_ID", strPtr("abc")),
			},
		},
	}
	cmd, entrypoint, env := azfSpecFromProps(props)
	if len(cmd) != 2 || cmd[0] != "echo" || cmd[1] != "hi" {
		t.Fatalf("cmd = %v", cmd)
	}
	if len(entrypoint) != 2 || entrypoint[0] != "/bin/sh" {
		t.Fatalf("entrypoint = %v", entrypoint)
	}
	if len(env) != 1 || env[0] != "APP_ENV=production" {
		t.Fatalf("env should be only the user var, got %v", env)
	}
}

func strPtr(s string) *string { return &s }
