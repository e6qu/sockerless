package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// CleanupItem represents a stale resource that can be cleaned up.
type CleanupItem struct {
	Category    string `json:"category"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Size        int64  `json:"size,omitempty"`
	Age         string `json:"age"`
}

// CleanupScanResult holds the results of a cleanup scan.
type CleanupScanResult struct {
	Items     []CleanupItem `json:"items"`
	ScannedAt string        `json:"scanned_at"`
}

// ScanOrphanedProcesses checks for stale PID files in ~/.sockerless/run/.
func ScanOrphanedProcesses() []CleanupItem {
	runDir := filepath.Join(sockerlessDir(), "run")
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil
	}

	var items []CleanupItem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".pid") {
			continue
		}

		pidPath := filepath.Join(runDir, e.Name())
		data, err := os.ReadFile(pidPath)
		if err != nil {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}

		// Check if process is alive — only ESRCH means the process is gone
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			// Process is dead — orphaned PID file
			info, _ := e.Info()
			age := ""
			if info != nil {
				age = formatDuration(time.Since(info.ModTime()))
			}
			items = append(items, CleanupItem{
				Category:    "process",
				Name:        e.Name(),
				Description: fmt.Sprintf("PID %d is no longer running", pid),
				Age:         age,
			})
		}
	}
	return items
}

// ScanStaleTmpFiles scans /tmp/sockerless-* dirs older than 1 hour.
func ScanStaleTmpFiles() []CleanupItem {
	entries, err := filepath.Glob("/tmp/sockerless-*")
	if err != nil {
		return nil
	}

	cutoff := time.Now().Add(-1 * time.Hour)
	var items []CleanupItem

	for _, entry := range entries {
		info, err := os.Stat(entry)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			size := dirSize(entry)
			items = append(items, CleanupItem{
				Category:    "tmp",
				Name:        filepath.Base(entry),
				Description: fmt.Sprintf("Temp directory (%s)", formatBytes(size)),
				Size:        size,
				Age:         formatDuration(time.Since(info.ModTime())),
			})
		}
	}
	return items
}

// ScanStoppedContainers queries backends for containers in exited/dead state.
func ScanStoppedContainers(reg *Registry, client *http.Client) []CleanupItem {
	backends := reg.ListByType("backend")
	if len(backends) == 0 {
		return nil
	}

	type result struct {
		items []CleanupItem
	}
	results := make(chan result, len(backends))
	var wg sync.WaitGroup

	for _, b := range backends {
		wg.Add(1)
		go func(comp Component) {
			defer wg.Done()
			body, status, err := proxyGET(client, comp.Addr, "/internal/v1/containers/summary")
			if err != nil || status != http.StatusOK {
				results <- result{}
				return
			}

			var containers []struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				State string `json:"state"`
			}
			if err := json.Unmarshal(body, &containers); err != nil {
				results <- result{}
				return
			}

			var items []CleanupItem
			for _, c := range containers {
				if c.State == "exited" || c.State == "dead" {
					shortID := c.ID
				if len(shortID) > 12 {
					shortID = shortID[:12]
				}
				items = append(items, CleanupItem{
						Category:    "container",
						Name:        c.Name,
						Description: fmt.Sprintf("Container %s on %s (state: %s)", shortID, comp.Name, c.State),
					})
				}
			}
			results <- result{items: items}
		}(b)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allItems []CleanupItem
	for r := range results {
		allItems = append(allItems, r.items...)
	}
	return allItems
}

// ScanStaleResources queries backends for resources not cleaned up after 1 hour.
func ScanStaleResources(reg *Registry, client *http.Client) []CleanupItem {
	backends := reg.ListByType("backend")
	if len(backends) == 0 {
		return nil
	}

	type result struct {
		items []CleanupItem
	}
	results := make(chan result, len(backends))
	var wg sync.WaitGroup

	for _, b := range backends {
		wg.Add(1)
		go func(comp Component) {
			defer wg.Done()
			body, status, err := proxyGET(client, comp.Addr, "/internal/v1/resources")
			if err != nil || status != http.StatusOK {
				results <- result{}
				return
			}

			var resources []struct {
				ResourceID   string `json:"resourceId"`
				ResourceType string `json:"resourceType"`
				CreatedAt    string `json:"createdAt"`
				CleanedUp    bool   `json:"cleanedUp"`
			}
			if err := json.Unmarshal(body, &resources); err != nil {
				results <- result{}
				return
			}

			cutoff := time.Now().Add(-1 * time.Hour)
			var items []CleanupItem
			for _, r := range resources {
				if r.CleanedUp {
					continue
				}
				created, err := time.Parse(time.RFC3339, r.CreatedAt)
				if err != nil {
					continue
				}
				if created.Before(cutoff) {
					items = append(items, CleanupItem{
						Category:    "resource",
						Name:        r.ResourceID,
						Description: fmt.Sprintf("%s on %s", r.ResourceType, comp.Name),
						Age:         formatDuration(time.Since(created)),
					})
				}
			}
			results <- result{items: items}
		}(b)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allItems []CleanupItem
	for r := range results {
		allItems = append(allItems, r.items...)
	}
	return allItems
}

// CleanOrphanedProcesses removes stale PID files.
func CleanOrphanedProcesses() int {
	items := ScanOrphanedProcesses()
	runDir := filepath.Join(sockerlessDir(), "run")
	cleaned := 0
	for _, item := range items {
		if err := os.Remove(filepath.Join(runDir, item.Name)); err == nil {
			cleaned++
		}
	}
	return cleaned
}

// CleanTmpFiles removes stale /tmp/sockerless-* directories.
func CleanTmpFiles() int {
	items := ScanStaleTmpFiles()
	cleaned := 0
	for _, item := range items {
		path := filepath.Join("/tmp", item.Name)
		if err := os.RemoveAll(path); err == nil {
			cleaned++
		}
	}
	return cleaned
}

// CleanStoppedContainers prunes stopped containers across backends.
func CleanStoppedContainers(reg *Registry, client *http.Client) int {
	backends := reg.ListByType("backend")
	cleaned := 0
	for _, b := range backends {
		_, status, err := proxyPOST(client, b.Addr, "/internal/v1/containers/prune")
		if err == nil && status == http.StatusOK {
			cleaned++
		}
	}
	return cleaned
}

// formatDuration formats a duration for human display.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// formatBytes formats bytes for human display.
func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// dirSize calculates the total size of a directory.
func dirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
