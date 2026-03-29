package core

// ContainerMetrics holds real resource usage metrics for a container.
type ContainerMetrics struct {
	CPUNanos int64 // Cumulative CPU usage in nanoseconds
	MemBytes int64 // Current memory usage in bytes
	PIDs     int   // Number of running processes
}

// StatsProvider fetches real resource metrics for a container.
// Cloud backends implement this to query their monitoring services
// (CloudWatch, Cloud Monitoring, Azure Monitor).
// Replaces all-zero synthetic stats with real values.
type StatsProvider interface {
	ContainerMetrics(containerID string) (*ContainerMetrics, error)
}
