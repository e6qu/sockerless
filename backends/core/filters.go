package core

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// MatchContainerFilters checks if a container matches the given filters.
func MatchContainerFilters(c api.Container, filters map[string][]string) bool {
	if filters == nil {
		return true
	}
	for key, values := range filters {
		switch key {
		case "id":
			matched := false
			for _, v := range values {
				if strings.HasPrefix(c.ID, v) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "name":
			matched := false
			for _, v := range values {
				name := strings.TrimPrefix(c.Name, "/")
				if name == v || c.Name == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "status":
			matched := false
			for _, v := range values {
				if c.State.Status == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "label":
			for _, v := range values {
				parts := strings.SplitN(v, "=", 2)
				if len(parts) == 2 {
					if c.Config.Labels[parts[0]] != parts[1] {
						return false
					}
				} else {
					if _, ok := c.Config.Labels[parts[0]]; !ok {
						return false
					}
				}
			}
		}
	}
	return true
}

// MatchNetworkFilters checks if a network matches the given filters.
func MatchNetworkFilters(n api.Network, filters map[string][]string) bool {
	if filters == nil {
		return true
	}
	for key, values := range filters {
		switch key {
		case "name":
			matched := false
			for _, v := range values {
				if n.Name == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "id":
			matched := false
			for _, v := range values {
				if strings.HasPrefix(n.ID, v) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "driver":
			matched := false
			for _, v := range values {
				if n.Driver == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "label":
			for _, v := range values {
				parts := strings.SplitN(v, "=", 2)
				if len(parts) == 2 {
					if n.Labels[parts[0]] != parts[1] {
						return false
					}
				} else {
					if _, ok := n.Labels[parts[0]]; !ok {
						return false
					}
				}
			}
		}
	}
	return true
}

// MatchNetworkPruneFilters checks if a network matches prune filters.
func MatchNetworkPruneFilters(n api.Network, filters map[string][]string) bool {
	if filters == nil {
		return true
	}
	for key, values := range filters {
		switch key {
		case "label":
			for _, v := range values {
				parts := strings.SplitN(v, "=", 2)
				if len(parts) == 2 {
					if n.Labels[parts[0]] != parts[1] {
						return false
					}
				} else {
					if _, ok := n.Labels[parts[0]]; !ok {
						return false
					}
				}
			}
		}
	}
	return true
}

// MatchVolumeFilters checks if a volume matches the given filters.
func MatchVolumeFilters(v api.Volume, filters map[string][]string) bool {
	if filters == nil {
		return true
	}
	for key, values := range filters {
		switch key {
		case "name":
			matched := false
			for _, val := range values {
				if v.Name == val {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "driver":
			matched := false
			for _, val := range values {
				if v.Driver == val {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "label":
			for _, val := range values {
				parts := strings.SplitN(val, "=", 2)
				if len(parts) == 2 {
					if v.Labels[parts[0]] != parts[1] {
						return false
					}
				} else {
					if _, ok := v.Labels[parts[0]]; !ok {
						return false
					}
				}
			}
		}
	}
	return true
}

// FormatStatus returns a human-readable status string for a container state.
func FormatStatus(state api.ContainerState) string {
	switch state.Status {
	case "created":
		return "Created"
	case "running":
		return "Up Less than a second"
	case "exited":
		if state.ExitCode == 0 {
			return "Exited (0)"
		}
		return fmt.Sprintf("Exited (%d)", state.ExitCode)
	case "dead":
		return "Dead"
	default:
		return state.Status
	}
}
