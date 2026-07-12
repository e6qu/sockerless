package bleephub

import (
	"strconv"
	"time"
)

// maxHookDeliveries bounds the retained delivery history per hook. Real GitHub
// exposes a recent, time-bounded window (≈30 days) rather than the full log;
// this cap keeps the in-memory (and persisted) history from growing without
// limit while retaining far more than any list page returns.
const maxHookDeliveries = 500

// Webhook represents a GitHub repository webhook.
//
// Secret carries a real json name so persistence round-trips it (deliveries
// must keep signing X-Hub-Signature-256 after a restart). Client responses
// never marshal this struct — hookToJSON emits an explicit map that omits
// the secret. RepoKey stays json:"-": it equals the persistence bucket key
// ("owner/name"), so the loader backfills it from the key on reload.
type Webhook struct {
	ID          int      `json:"id"`
	URL         string   `json:"config_url"`
	Secret      string   `json:"secret"`
	ContentType string   `json:"content_type"`
	InsecureSSL string   `json:"insecure_ssl"`
	Events      []string `json:"events"`
	Active      bool     `json:"active"`
	RepoKey     string   `json:"-"`
	// OrgLogin marks an organization-level hook; like RepoKey it equals
	// the persistence bucket key, so it stays json:"-" and the loader
	// backfills it on reload. Exactly one of RepoKey/OrgLogin is set
	// (both empty = app-level pseudo-hook).
	OrgLogin string `json:"-"`
	// MarketplaceSlug marks the listing-owned Marketplace webhook. It is an
	// ephemeral delivery coordinate; listing persistence owns the configuration.
	MarketplaceSlug string `json:"-"`
	// LastResponse mirrors GitHub's hook.last_response: the outcome of the most
	// recent delivery. Nil until a delivery has occurred (rendered "unused").
	LastResponse *HookLastResponse `json:"last_response,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// HookLastResponse is the outcome of a webhook's most recent delivery.
type HookLastResponse struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// DeliveryRequest holds the request details of a webhook delivery.
type DeliveryRequest struct {
	Headers map[string]string `json:"headers"`
	Payload interface{}       `json:"payload"`
}

// DeliveryResponse holds the response details of a webhook delivery.
type DeliveryResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// WebhookDelivery records a single delivery attempt for a webhook.
type WebhookDelivery struct {
	ID             int               `json:"id"`
	HookID         int               `json:"hook_id"`
	AppID          int               `json:"app_id,omitempty"`
	InstallationID int               `json:"installation_id,omitempty"`
	RepositoryID   int               `json:"repository_id,omitempty"`
	TargetURL      string            `json:"url"`
	GUID           string            `json:"guid"`
	Event          string            `json:"event"`
	Action         string            `json:"action"`
	StatusCode     int               `json:"status_code"`
	Duration       float64           `json:"duration"`
	Request        *DeliveryRequest  `json:"request"`
	Response       *DeliveryResponse `json:"response"`
	Redelivery     bool              `json:"redelivery"`
	DeliveredAt    time.Time         `json:"delivered_at"`
	ThrottledAt    *time.Time        `json:"throttled_at"`
}

// CreateHook creates a new webhook for a repository.
func (st *Store) CreateHook(repoKey, url, secret, contentType, insecureSSL string, events []string, active bool) *Webhook {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.Hooks == nil {
		st.Hooks = make(map[string][]*Webhook)
	}

	if contentType == "" {
		contentType = "form"
	}
	if insecureSSL == "" {
		insecureSSL = "0"
	}

	now := time.Now()
	hook := &Webhook{
		ID:          st.NextHookID,
		URL:         url,
		Secret:      secret,
		ContentType: contentType,
		InsecureSSL: insecureSSL,
		Events:      events,
		Active:      active,
		RepoKey:     repoKey,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextHookID++
	st.Hooks[repoKey] = append(st.Hooks[repoKey], hook)
	if st.persist != nil {
		st.persist.MustPut("hooks", repoKey, st.Hooks[repoKey])
	}
	return hook
}

// GetHook returns a webhook by repo key and hook ID, or nil.
func (st *Store) GetHook(repoKey string, hookID int) *Webhook {
	st.mu.RLock()
	defer st.mu.RUnlock()

	for _, h := range st.Hooks[repoKey] {
		if h.ID == hookID {
			return h
		}
	}
	return nil
}

// ListHooks returns all webhooks for a repository.
func (st *Store) ListHooks(repoKey string) []*Webhook {
	st.mu.RLock()
	defer st.mu.RUnlock()

	hooks := st.Hooks[repoKey]
	out := make([]*Webhook, len(hooks))
	copy(out, hooks)
	return out
}

// UpdateHook updates a webhook in place. Returns false if not found.
func (st *Store) UpdateHook(repoKey string, hookID int, fn func(h *Webhook)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, h := range st.Hooks[repoKey] {
		if h.ID == hookID {
			fn(h)
			h.UpdatedAt = time.Now()
			if st.persist != nil {
				st.persist.MustPut("hooks", repoKey, st.Hooks[repoKey])
			}
			return true
		}
	}
	return false
}

// DeleteHook removes a webhook. Returns false if not found.
func (st *Store) DeleteHook(repoKey string, hookID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	hooks := st.Hooks[repoKey]
	for i, h := range hooks {
		if h.ID == hookID {
			st.Hooks[repoKey] = append(hooks[:i], hooks[i+1:]...)
			if st.persist != nil {
				if len(st.Hooks[repoKey]) > 0 {
					st.persist.MustPut("hooks", repoKey, st.Hooks[repoKey])
				} else {
					st.persist.MustDelete("hooks", repoKey)
				}
			}
			return true
		}
	}
	return false
}

// AddDelivery records a webhook delivery.
func (st *Store) AddDelivery(delivery *WebhookDelivery) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.HookDeliveries == nil {
		st.HookDeliveries = make(map[int][]*WebhookDelivery)
	}

	delivery.ID = st.NextDeliveryID
	st.NextDeliveryID++
	list := append(st.HookDeliveries[delivery.HookID], delivery)
	// GitHub retains only a recent window of webhook deliveries (≈30 days),
	// not the full history. Bound the per-hook slice so a hook pointed at a
	// dead endpoint (3 delivery records per event, forever) cannot grow the
	// store without limit. Keep the newest maxHookDeliveries.
	if len(list) > maxHookDeliveries {
		list = list[len(list)-maxHookDeliveries:]
	}
	st.HookDeliveries[delivery.HookID] = list
	if st.persist != nil {
		st.persist.MustPut("hook_deliveries", strconv.Itoa(delivery.HookID), list)
	}
}

// HookLastResp returns the hook's last_response pointer read under the store
// lock. Reading h.LastResponse directly races SetHookLastResponse's write,
// which runs on the async deliverWebhook goroutine; this snapshot is the
// synchronized read every JSON-rendering path must use.
func (st *Store) HookLastResp(h *Webhook) *HookLastResponse {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return h.LastResponse
}

// SetHookLastResponse records the outcome of a hook's most recent delivery so
// the hook object's last_response field reflects real delivery results.
func (st *Store) SetHookLastResponse(repoKey string, hookID int, lr *HookLastResponse) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, h := range st.Hooks[repoKey] {
		if h.ID == hookID {
			h.LastResponse = lr
			if st.persist != nil {
				st.persist.MustPut("hooks", repoKey, st.Hooks[repoKey])
			}
			return
		}
	}
}

// ListDeliveries returns all deliveries for a webhook, newest first.
func (st *Store) ListDeliveries(hookID int) []*WebhookDelivery {
	st.mu.RLock()
	defer st.mu.RUnlock()

	deliveries := st.HookDeliveries[hookID]
	out := make([]*WebhookDelivery, len(deliveries))
	copy(out, deliveries)
	// Reverse for newest-first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
