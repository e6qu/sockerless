package bleephub

import "time"

// Webhook represents a GitHub repository webhook.
type Webhook struct {
	ID        int       `json:"id"`
	URL       string    `json:"config_url"`
	Secret    string    `json:"-"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	RepoKey   string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	ID          int               `json:"id"`
	HookID      int               `json:"hook_id"`
	GUID        string            `json:"guid"`
	Event       string            `json:"event"`
	Action      string            `json:"action"`
	StatusCode  int               `json:"status_code"`
	Duration    float64           `json:"duration"`
	Request     *DeliveryRequest  `json:"request"`
	Response    *DeliveryResponse `json:"response"`
	Redelivery  bool              `json:"redelivery"`
	DeliveredAt time.Time         `json:"delivered_at"`
}

// CreateHook creates a new webhook for a repository.
func (st *Store) CreateHook(repoKey, url, secret string, events []string, active bool) *Webhook {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.Hooks == nil {
		st.Hooks = make(map[string][]*Webhook)
	}

	now := time.Now()
	hook := &Webhook{
		ID:        st.NextHookID,
		URL:       url,
		Secret:    secret,
		Events:    events,
		Active:    active,
		RepoKey:   repoKey,
		CreatedAt: now,
		UpdatedAt: now,
	}
	st.NextHookID++
	st.Hooks[repoKey] = append(st.Hooks[repoKey], hook)
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
	st.HookDeliveries[delivery.HookID] = append(st.HookDeliveries[delivery.HookID], delivery)
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
