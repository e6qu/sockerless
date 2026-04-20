package ecs

import (
	"errors"
	"testing"
	"time"

	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// BUG-709: pollOperation must sleep between polls so that 60 attempts
// span ~120s of wall time, not <1s of back-to-back API calls.
func TestPollOperation_SleepsBetweenAttempts(t *testing.T) {
	var sleeps []time.Duration
	calls := 0
	poll := func(_ string) (sdtypes.OperationStatus, string, error) {
		calls++
		if calls < 4 {
			return sdtypes.OperationStatusPending, "", nil
		}
		return sdtypes.OperationStatusSuccess, "ns-abc", nil
	}
	got, err := pollOperation("op-1", 2*time.Second, 60, func(d time.Duration) { sleeps = append(sleeps, d) }, poll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ns-abc" {
		t.Fatalf("got namespace %q, want ns-abc", got)
	}
	if calls != 4 {
		t.Fatalf("got %d polls, want 4", calls)
	}
	// 3 PENDING attempts → 3 sleeps of 2s each. SUCCESS terminates without sleeping.
	if len(sleeps) != 3 {
		t.Fatalf("got %d sleeps, want 3", len(sleeps))
	}
	for _, s := range sleeps {
		if s != 2*time.Second {
			t.Fatalf("got sleep %v, want 2s", s)
		}
	}
}

func TestPollOperation_PropagatesAPIError(t *testing.T) {
	apiErr := errors.New("aws boom")
	got, err := pollOperation("op-1", time.Millisecond, 60, func(time.Duration) {}, func(string) (sdtypes.OperationStatus, string, error) {
		return "", "", apiErr
	})
	if got != "" || !errors.Is(err, apiErr) {
		t.Fatalf("expected to propagate API error, got got=%q err=%v", got, err)
	}
}

func TestPollOperation_FailStatus(t *testing.T) {
	_, err := pollOperation("op-1", time.Millisecond, 60, func(time.Duration) {}, func(string) (sdtypes.OperationStatus, string, error) {
		return sdtypes.OperationStatusFail, "ConflictingDomainExists", nil
	})
	if err == nil || !contains(err.Error(), "ConflictingDomainExists") {
		t.Fatalf("expected error mentioning fail reason, got %v", err)
	}
}

func TestPollOperation_TimeoutAfterMaxAttempts(t *testing.T) {
	calls := 0
	_, err := pollOperation("op-1", time.Millisecond, 5, func(time.Duration) {}, func(string) (sdtypes.OperationStatus, string, error) {
		calls++
		return sdtypes.OperationStatusPending, "", nil
	})
	if err == nil || !contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if calls != 5 {
		t.Fatalf("got %d polls, want 5", calls)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// serverWithNetworks returns a Server wired only with a NetworkState so
// searchDomainsForContainer can be tested without AWS clients.
func serverWithNetworks(entries map[string]NetworkState) *Server {
	s := &Server{
		NetworkState: core.NewStateStore[NetworkState](),
	}
	for id, ns := range entries {
		s.NetworkState.Put(id, ns)
	}
	return s
}

func TestSearchDomainsForContainer_Nil(t *testing.T) {
	s := serverWithNetworks(nil)
	if got := s.searchDomainsForContainer(nil); got != nil {
		t.Fatalf("expected nil for nil container, got %v", got)
	}
}

func TestSearchDomainsForContainer_SkipsPredefinedNetworks(t *testing.T) {
	s := serverWithNetworks(nil)
	c := &api.Container{
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"bridge": {NetworkID: "nid-bridge"},
				"host":   {NetworkID: "nid-host"},
				"none":   {NetworkID: "nid-none"},
			},
		},
	}
	if got := s.searchDomainsForContainer(c); len(got) != 0 {
		t.Fatalf("expected no domains for bridge/host/none, got %v", got)
	}
}

func TestSearchDomainsForContainer_SkipsNetworksWithoutNamespace(t *testing.T) {
	s := serverWithNetworks(map[string]NetworkState{
		"nid-foo": {NamespaceID: ""}, // namespace not registered
	})
	c := &api.Container{
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"foo": {NetworkID: "nid-foo"},
			},
		},
	}
	if got := s.searchDomainsForContainer(c); len(got) != 0 {
		t.Fatalf("expected no domains when namespace missing, got %v", got)
	}
}

func TestSearchDomainsForContainer_SkipsEmptyNetworkID(t *testing.T) {
	s := serverWithNetworks(nil)
	c := &api.Container{
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"foo": {NetworkID: ""},
				"bar": nil,
			},
		},
	}
	if got := s.searchDomainsForContainer(c); len(got) != 0 {
		t.Fatalf("expected no domains, got %v", got)
	}
}
