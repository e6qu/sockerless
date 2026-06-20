package bleephub

import (
	"net/http/httptest"
	"sync"
	"testing"
)

// TestHookLastResponseRace drives SetHookLastResponse (the async
// deliverWebhook write path) concurrently with the JSON-rendering read path.
// Under -race this fails if last_response is read off the shared *Webhook
// without the store lock. The fix routes every read through Store.HookLastResp.
func TestHookLastResponseRace(t *testing.T) {
	s := newTestServer()
	repoKey := "octo/repo"
	hook := s.store.CreateHook(repoKey, "http://example.invalid/hook", "", "json", "0", []string{"push"}, true)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			s.store.SetHookLastResponse(repoKey, hook.ID, &HookLastResponse{Code: 200 + n, Status: "ok"})
		}(i)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("GET", "/api/v3/repos/octo/repo/hooks", nil)
			_ = hookToJSON(hook, s.store.HookLastResp(hook), r, "octo", "repo")
		}()
	}
	wg.Wait()
}
