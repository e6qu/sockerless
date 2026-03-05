package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
		case "ancestor":
			matched := false
			for _, v := range values {
				if c.Config.Image == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "network":
			matched := false
			for _, v := range values {
				for netName := range c.NetworkSettings.Networks {
					if netName == v {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				return false
			}
		case "health":
			status := "none"
			if c.State.Health != nil {
				status = c.State.Health.Status
			}
			matched := false
			for _, v := range values {
				if status == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "exited": // BUG-385
			if c.State.Status != "exited" {
				return false
			}
			matched := false
			for _, v := range values {
				code, err := strconv.Atoi(v)
				if err == nil && c.State.ExitCode == code {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "publish": // BUG-391
			matched := false
			for _, v := range values {
				for port := range c.HostConfig.PortBindings {
					if string(port) == v || strings.HasPrefix(string(port), v+"/") {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				return false
			}
		case "volume": // BUG-392
			matched := false
			for _, v := range values {
				for _, m := range c.Mounts {
					if m.Name == v || m.Source == v || m.Destination == v {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				return false
			}
		case "expose": // BUG-489
			matched := false
			for _, val := range values {
				if _, ok := c.Config.ExposedPorts[val]; ok {
					matched = true
					break
				}
				// Try with /tcp suffix if no protocol specified
				if !strings.Contains(val, "/") {
					if _, ok := c.Config.ExposedPorts[val+"/tcp"]; ok {
						matched = true
						break
					}
				}
			}
			if !matched {
				return false
			}
		case "is-task": // BUG-393
			for _, v := range values {
				if v == "true" {
					return false // No Swarm tasks in sockerless
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
		started, err := time.Parse(time.RFC3339Nano, state.StartedAt)
		if err != nil {
			return "Up Less than a second"
		}
		d := time.Since(started)
		switch {
		case d < time.Second:
			return "Up Less than a second"
		case d < time.Minute:
			return fmt.Sprintf("Up %d seconds", int(d.Seconds()))
		case d < time.Hour:
			return fmt.Sprintf("Up %d minutes", int(d.Minutes()))
		case d < 24*time.Hour:
			return fmt.Sprintf("Up %d hours", int(d.Hours()))
		default:
			return fmt.Sprintf("Up %d days", int(d.Hours()/24))
		}
	case "paused":
		started, err := time.Parse(time.RFC3339Nano, state.StartedAt)
		if err != nil {
			return "Up Less than a second (Paused)"
		}
		d := time.Since(started)
		switch {
		case d < time.Second:
			return "Up Less than a second (Paused)"
		case d < time.Minute:
			return fmt.Sprintf("Up %d seconds (Paused)", int(d.Seconds()))
		case d < time.Hour:
			return fmt.Sprintf("Up %d minutes (Paused)", int(d.Minutes()))
		case d < 24*time.Hour:
			return fmt.Sprintf("Up %d hours (Paused)", int(d.Hours()))
		default:
			return fmt.Sprintf("Up %d days (Paused)", int(d.Hours()/24))
		}
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
