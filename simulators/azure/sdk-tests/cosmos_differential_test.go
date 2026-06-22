package azure_sdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

// Differential testing of the Cosmos slice against Microsoft's own Cosmos DB
// emulator (the vNext Linux emulator in HTTP mode). The SAME azcosmos client code
// runs every scenario against the sim and the emulator, differing only in the
// endpoint, and the observable outcome must match. As with the DynamoDB-Local and
// Firestore differentials, the emulator is a REFERENCE, not a ceiling: a case
// where the sim is intentionally more faithful is recorded in
// cosmosDiffKnownDivergences and asserted exactly — the sim is never regressed to
// match the oracle.
//
// The emulator is large and slow to start, so this is Docker-gated: it runs for
// real where Docker + the image are available (CI pre-pulls it) and skips with
// diagnostics otherwise, never flaking the suite.

type cosmosDiffResult struct {
	value   string // canonical JSON of the observable (user fields only), or sentinel
	errCode string // HTTP status of an azcosmos error, "" on success
}

type cosmosDiffDivergence struct {
	sim    cosmosDiffResult
	oracle cosmosDiffResult
	reason string
}

// Empty today: every scenario below agrees with the emulator.
var cosmosDiffKnownDivergences = map[string]cosmosDiffDivergence{}

func TestCosmos_DifferentialVsEmulator(t *testing.T) {
	emuEndpoint, stop := startCosmosEmulator(t)
	defer stop()

	simC := newCosmosSDKClient(t, baseURL+"/")
	oracle := newCosmosSDKClient(t, emuEndpoint)

	for _, sc := range cosmosDiffScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			simRes := cosmosCapture(sc.run(t, simC, "sim-"+sc.name))
			oracleRes := cosmosCapture(sc.run(t, oracle, "emu-"+sc.name))
			if div, ok := cosmosDiffKnownDivergences[sc.name]; ok {
				cosmosAssertEqual(t, "sim (documented divergence: "+div.reason+")", div.sim, simRes)
				cosmosAssertEqual(t, "emulator (documented divergence: "+div.reason+")", div.oracle, oracleRes)
				return
			}
			if simRes != oracleRes {
				t.Errorf("differential mismatch for %q:\n  sim:      %+v\n  emulator: %+v\n"+
					"If the sim is the MORE faithful one, add a cosmosDiffKnownDivergences entry (do not regress the sim).",
					sc.name, simRes, oracleRes)
			}
		})
	}
}

type cosmosDiffScenario struct {
	name string
	run  func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error)
}

func cosmosCapture(data map[string]any, err error) cosmosDiffResult {
	if err != nil {
		var re *azcore.ResponseError
		if errors.As(err, &re) {
			return cosmosDiffResult{errCode: fmt.Sprintf("%d", re.StatusCode)}
		}
		return cosmosDiffResult{errCode: "error:" + err.Error()}
	}
	return cosmosDiffResult{value: cosmosCanon(data)}
}

func cosmosAssertEqual(t *testing.T, side string, want, got cosmosDiffResult) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %+v, got %+v", side, want, got)
	}
}

func cosmosDiffScenarios() []cosmosDiffScenario {
	pk := azcosmos.NewPartitionKeyString("p")
	makeContainer := func(t *testing.T, c *azcosmos.Client, dbID string) *azcosmos.ContainerClient {
		t.Helper()
		if _, err := c.CreateDatabase(ctx, azcosmos.DatabaseProperties{ID: dbID}, nil); err != nil {
			t.Fatalf("CreateDatabase: %v", err)
		}
		db, _ := c.NewDatabase(dbID)
		if _, err := db.CreateContainer(ctx, azcosmos.ContainerProperties{
			ID:                     "c",
			PartitionKeyDefinition: azcosmos.PartitionKeyDefinition{Paths: []string{"/pk"}},
		}, nil); err != nil {
			t.Fatalf("CreateContainer: %v", err)
		}
		cc, _ := c.NewContainer(dbID, "c")
		return cc
	}
	create := func(cc *azcosmos.ContainerClient, doc map[string]any) error {
		raw, _ := json.Marshal(doc)
		_, err := cc.CreateItem(ctx, pk, raw, nil)
		return err
	}

	return []cosmosDiffScenario{
		{"create-read-roundtrip", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			if err := create(cc, map[string]any{"id": "x", "pk": "p", "name": "alice", "age": 30}); err != nil {
				return nil, err
			}
			resp, err := cc.ReadItem(ctx, pk, "x", nil)
			if err != nil {
				return nil, err
			}
			return cosmosUnmarshal(resp.Value), nil
		}},
		{"upsert-replaces", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			if err := create(cc, map[string]any{"id": "x", "pk": "p", "v": 1}); err != nil {
				return nil, err
			}
			raw, _ := json.Marshal(map[string]any{"id": "x", "pk": "p", "v": 2})
			if _, err := cc.UpsertItem(ctx, pk, raw, nil); err != nil {
				return nil, err
			}
			resp, err := cc.ReadItem(ctx, pk, "x", nil)
			if err != nil {
				return nil, err
			}
			return cosmosUnmarshal(resp.Value), nil
		}},
		{"create-conflict-409", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			if err := create(cc, map[string]any{"id": "x", "pk": "p"}); err != nil {
				return nil, err
			}
			return nil, create(cc, map[string]any{"id": "x", "pk": "p"}) // duplicate id → 409
		}},
		{"read-missing-404", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			_, err := cc.ReadItem(ctx, pk, "nope", nil)
			return nil, err
		}},
		{"patch-increment", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			if err := create(cc, map[string]any{"id": "x", "pk": "p", "count": 10}); err != nil {
				return nil, err
			}
			patch := azcosmos.PatchOperations{}
			patch.AppendIncrement("/count", 5)
			patch.AppendSet("/status", "active")
			if _, err := cc.PatchItem(ctx, pk, "x", patch, nil); err != nil {
				return nil, err
			}
			resp, err := cc.ReadItem(ctx, pk, "x", nil)
			if err != nil {
				return nil, err
			}
			return cosmosUnmarshal(resp.Value), nil
		}},
		{"delete-then-read-404", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			if err := create(cc, map[string]any{"id": "x", "pk": "p"}); err != nil {
				return nil, err
			}
			if _, err := cc.DeleteItem(ctx, pk, "x", nil); err != nil {
				return nil, err
			}
			_, err := cc.ReadItem(ctx, pk, "x", nil)
			return nil, err
		}},
		{"query-where-and-order", func(t *testing.T, c *azcosmos.Client, dbID string) (map[string]any, error) {
			cc := makeContainer(t, c, dbID)
			for i, d := range []map[string]any{
				{"id": "a", "pk": "p", "team": "x", "score": 5},
				{"id": "b", "pk": "p", "team": "x", "score": 10},
				{"id": "c", "pk": "p", "team": "y", "score": 7},
			} {
				if err := create(cc, d); err != nil {
					return nil, fmt.Errorf("seed %d: %w", i, err)
				}
			}
			pager := cc.NewQueryItemsPager(
				"SELECT c.id FROM c WHERE c.team = @t ORDER BY c.score DESC", pk,
				&azcosmos.QueryOptions{QueryParameters: []azcosmos.QueryParameter{{Name: "@t", Value: "x"}}})
			var ids []any
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, it := range page.Items {
					m := cosmosUnmarshal(it)
					ids = append(ids, m["id"])
				}
			}
			return map[string]any{"ids": ids}, nil // ORDER BY score DESC → [b, a]
		}},
	}
}

// ── normalization ────────────────────────────────────────────────────────────

func cosmosUnmarshal(b []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// cosmosCanon renders the observable comparably, dropping Cosmos system fields
// (_rid/_self/_etag/_ts/_attachments) that legitimately differ between the sim
// and the emulator, and normalizing numbers so int vs float formatting doesn't
// cause false mismatches.
func cosmosCanon(data map[string]any) string {
	b, _ := json.Marshal(cosmosCanonValue(stripCosmosSystemFields(data)))
	return string(b)
}

func stripCosmosSystemFields(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		out[k] = v
	}
	return out
}

func cosmosCanonValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([][2]any, 0, len(t))
		for _, k := range keys {
			out = append(out, [2]any{k, cosmosCanonValue(t[k])})
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = cosmosCanonValue(e)
		}
		return out
	case float64:
		// JSON numbers decode as float64; render without trailing-zero noise.
		return fmt.Sprintf("%g", t)
	default:
		return t
	}
}

// ── emulator lifecycle ───────────────────────────────────────────────────────

func startCosmosEmulator(t *testing.T) (endpoint string, stop func()) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available; skipping Cosmos emulator differential test")
	}
	const image = "mcr.microsoft.com/cosmosdb/linux/azure-cosmos-emulator:vnext-preview"
	if exec.Command("docker", "image", "inspect", image).Run() != nil {
		// Do not pull a ~GB image inline; CI pre-pulls it. Skip with diagnostics.
		t.Skipf("Cosmos emulator image %s not present locally (CI pre-pulls it); skipping", image)
	}

	port := freeTCPPort(t)
	runOut, err := exec.Command("docker", "run", "-d", "--rm",
		"-p", fmt.Sprintf("127.0.0.1:%d:8081", port),
		image, "--protocol", "http").CombinedOutput()
	if err != nil {
		t.Skipf("could not start Cosmos emulator (skipping differential): %v\n%s", err, runOut)
	}
	id := trimSpace(string(runOut))
	stop = func() { _ = exec.Command("docker", "rm", "-f", id).Run() }

	endpoint = fmt.Sprintf("http://127.0.0.1:%d/", port)
	probe := newCosmosSDKClient(t, endpoint)
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, perr := probe.CreateDatabase(cctx, azcosmos.DatabaseProperties{ID: "readyprobe"}, nil)
		cancel()
		if perr == nil {
			return endpoint, stop
		}
		time.Sleep(2 * time.Second)
	}
	logs, _ := exec.Command("docker", "logs", "--tail", "30", id).CombinedOutput()
	stop()
	t.Skipf("Cosmos emulator did not become ready at %s (skipping differential)\n%s", endpoint, logs)
	return "", func() {}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
