package core

import (
	"reflect"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestEnvReaders(t *testing.T) {
	t.Setenv("SOCKERLESS_TEST_STR", "value")
	t.Setenv("SOCKERLESS_TEST_INT", "42")
	t.Setenv("SOCKERLESS_TEST_BAD_INT", "forty-two")
	t.Setenv("SOCKERLESS_TEST_EMPTY", "")
	t.Setenv("SOCKERLESS_TEST_KIND", " host-aliases ")
	t.Setenv("SOCKERLESS_TEST_ACCESS", "azure-ad")

	if got := EnvOrDefault("SOCKERLESS_TEST_STR", "def"); got != "value" {
		t.Errorf("EnvOrDefault set = %q", got)
	}
	if got := EnvOrDefault("SOCKERLESS_TEST_EMPTY", "def"); got != "def" {
		t.Errorf("EnvOrDefault empty = %q", got)
	}
	if got := EnvOrDefaultInt("SOCKERLESS_TEST_INT", 7); got != 42 {
		t.Errorf("EnvOrDefaultInt set = %d", got)
	}
	if got := EnvOrDefaultInt("SOCKERLESS_TEST_BAD_INT", 7); got != 7 {
		t.Errorf("EnvOrDefaultInt malformed = %d, want default", got)
	}
	if got := EnvOrDefaultInt("SOCKERLESS_TEST_UNSET", 7); got != 7 {
		t.Errorf("EnvOrDefaultInt unset = %d, want default", got)
	}
	if got := NetworkDiscoveryFromEnv("SOCKERLESS_TEST_KIND", api.NetworkDiscoveryCloudDNS); got != api.NetworkDiscoveryHostAliases {
		t.Errorf("NetworkDiscoveryFromEnv = %q (must trim)", got)
	}
	if got := NetworkDiscoveryFromEnv("SOCKERLESS_TEST_UNSET", api.NetworkDiscoveryCloudDNS); got != api.NetworkDiscoveryCloudDNS {
		t.Errorf("NetworkDiscoveryFromEnv unset = %q, want default", got)
	}
	if got := AccessFromEnv("SOCKERLESS_TEST_ACCESS", api.AccessMechanismNoneInternal); got != api.AccessMechanismAzureAD {
		t.Errorf("AccessFromEnv = %q", got)
	}
	if got := AccessFromEnv("SOCKERLESS_TEST_UNSET", api.AccessMechanismNoneInternal); got != api.AccessMechanismNoneInternal {
		t.Errorf("AccessFromEnv unset = %q, want default", got)
	}
}

func TestDurationOrDefault(t *testing.T) {
	if got := DurationOrDefault("", time.Second); got != time.Second {
		t.Errorf("empty = %s", got)
	}
	if got := DurationOrDefault("5s", time.Second); got != 5*time.Second {
		t.Errorf("5s = %s", got)
	}
	if got := DurationOrDefault("soon", time.Second); got != time.Second {
		t.Errorf("malformed = %s, want default", got)
	}
}

func TestSplitCSV(t *testing.T) {
	if got := SplitCSV(""); got != nil {
		t.Errorf("empty = %v, want nil", got)
	}
	if got, want := SplitCSV(" a, b ,,c "), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SplitCSV = %v, want %v", got, want)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "", "x", "y"); got != "x" {
		t.Errorf("FirstNonEmpty = %q", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Errorf("FirstNonEmpty all empty = %q", got)
	}
}
