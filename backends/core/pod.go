package core

import (
	"strings"
	"sync"
	"time"
)

// PodContext tracks containers that should run in a single cloud task/job.
type PodContext struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	ContainerIDs []string          `json:"containerIds"`
	StartedIDs   []string          `json:"startedIds,omitempty"` // containers that have been started
	NetworkName  string            `json:"networkName,omitempty"`
	Status       string            `json:"status"` // "created", "running", "stopped", "exited"
	Labels       map[string]string `json:"labels,omitempty"`
	Hostname     string            `json:"hostname,omitempty"`
	SharedNS     []string          `json:"sharedNs,omitempty"` // default: ["ipc","net","uts"]
	InfraID      string            `json:"infraId,omitempty"`
	Created      string            `json:"created"`
}

// PodRegistry tracks pods with O(1) lookups by name, container ID, and network.
type PodRegistry struct {
	mu          sync.RWMutex
	pods        map[string]*PodContext // podID → PodContext
	byName      map[string]string      // podName → podID
	byContainer map[string]string      // containerID → podID
	byNetwork   map[string]string      // networkName → podID
}

// NewPodRegistry creates a new empty PodRegistry.
func NewPodRegistry() *PodRegistry {
	return &PodRegistry{
		pods:        make(map[string]*PodContext),
		byName:      make(map[string]string),
		byContainer: make(map[string]string),
		byNetwork:   make(map[string]string),
	}
}

// CreatePod creates a new pod with the given name and labels.
func (pr *PodRegistry) CreatePod(name string, labels map[string]string) *PodContext {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	id := GenerateID()
	if labels == nil {
		labels = make(map[string]string)
	}
	pod := &PodContext{
		ID:       id,
		Name:     name,
		Status:   "created",
		Labels:   labels,
		SharedNS: []string{"ipc", "net", "uts"},
		Created:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	pr.pods[id] = pod
	pr.byName[name] = id
	return pod
}

// AddContainer adds a container to the pod. Idempotent — adding the same
// container twice is a no-op.
func (pr *PodRegistry) AddContainer(podID, containerID string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pod, ok := pr.pods[podID]
	if !ok {
		return &PodNotFoundError{ID: podID}
	}

	// Check if already in this pod
	for _, cid := range pod.ContainerIDs {
		if cid == containerID {
			return nil
		}
	}

	pod.ContainerIDs = append(pod.ContainerIDs, containerID)
	pr.byContainer[containerID] = podID
	return nil
}

// GetPod looks up a pod by ID first, then by name.
func (pr *PodRegistry) GetPod(nameOrID string) (*PodContext, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	if pod, ok := pr.pods[nameOrID]; ok {
		return pod, true
	}
	if podID, ok := pr.byName[nameOrID]; ok {
		return pr.pods[podID], true
	}
	// Prefix match on ID
	for id, pod := range pr.pods {
		if strings.HasPrefix(id, nameOrID) {
			return pod, true
		}
	}
	return nil, false
}

// GetPodForContainer returns the pod containing the given container ID.
func (pr *PodRegistry) GetPodForContainer(containerID string) (*PodContext, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	podID, ok := pr.byContainer[containerID]
	if !ok {
		return nil, false
	}
	return pr.pods[podID], true
}

// GetPodForNetwork returns the pod associated with the given network name.
func (pr *PodRegistry) GetPodForNetwork(networkName string) (*PodContext, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	podID, ok := pr.byNetwork[networkName]
	if !ok {
		return nil, false
	}
	return pr.pods[podID], true
}

// SetNetwork associates a network with a pod.
func (pr *PodRegistry) SetNetwork(podID, networkName string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if pod, ok := pr.pods[podID]; ok {
		pod.NetworkName = networkName
		pr.byNetwork[networkName] = podID
	}
}

// SetStatus updates the pod status.
func (pr *PodRegistry) SetStatus(podID, status string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if pod, ok := pr.pods[podID]; ok {
		pod.Status = status
	}
}

// MarkStarted records that a container has been started in its pod.
// Returns (shouldDefer, podContainerIDs):
//   - shouldDefer=true: not all containers started yet
//   - shouldDefer=false: all started, podContainerIDs has the full list
func (pr *PodRegistry) MarkStarted(podID, containerID string) (bool, []string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pod, ok := pr.pods[podID]
	if !ok {
		return false, nil
	}

	// Idempotent: skip if already in StartedIDs
	for _, sid := range pod.StartedIDs {
		if sid == containerID {
			return len(pod.StartedIDs) < len(pod.ContainerIDs), append([]string{}, pod.ContainerIDs...)
		}
	}

	pod.StartedIDs = append(pod.StartedIDs, containerID)
	return len(pod.StartedIDs) < len(pod.ContainerIDs), append([]string{}, pod.ContainerIDs...)
}

// ListPods returns all pods.
func (pr *PodRegistry) ListPods() []*PodContext {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	result := make([]*PodContext, 0, len(pr.pods))
	for _, pod := range pr.pods {
		result = append(result, pod)
	}
	return result
}

// DeletePod removes a pod and all its index entries.
func (pr *PodRegistry) DeletePod(podID string) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pod, ok := pr.pods[podID]
	if !ok {
		return false
	}

	// Remove container index entries
	for _, cid := range pod.ContainerIDs {
		delete(pr.byContainer, cid)
	}

	// Remove network index
	if pod.NetworkName != "" {
		delete(pr.byNetwork, pod.NetworkName)
	}

	// Remove name index
	delete(pr.byName, pod.Name)

	// Remove pod
	delete(pr.pods, podID)
	return true
}

// Exists checks if a pod with the given name or ID exists.
func (pr *PodRegistry) Exists(nameOrID string) bool {
	_, ok := pr.GetPod(nameOrID)
	return ok
}

// PodNotFoundError indicates a pod was not found.
type PodNotFoundError struct {
	ID string
}

func (e *PodNotFoundError) Error() string {
	return "pod not found: " + e.ID
}
