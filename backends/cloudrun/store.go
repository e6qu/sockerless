package cloudrun

// CloudRunState maps sockerless container IDs to GCP resource names.
// Jobs and Services are mutually exclusive at the backend level (chosen
// by Config.UseService), but kept in the same struct so the cache layer
// doesn't need to branch on execution model.
type CloudRunState struct {
	JobName       string // Cloud Run Job name (UseService=false path)
	ExecutionName string // Execution name (UseService=false path)
	ServiceName   string // Cloud Run Service name (UseService=true path)
}

// NetworkState tracks cloud networking state for a Docker network.
type NetworkState struct {
	ManagedZoneName string // Cloud DNS managed zone name
	DNSName         string // DNS zone name (e.g., "network-name.internal.")
}
