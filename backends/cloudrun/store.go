package cloudrun

// CloudRunState maps sockerless container IDs to GCP resource names.
type CloudRunState struct {
	JobName       string // Cloud Run Job name
	ExecutionName string // Execution name
	AgentAddress  string // "ip:port" of the agent
	AgentToken    string // Bearer token for agent auth
}

// NetworkState tracks cloud networking state for a Docker network.
type NetworkState struct {
	ManagedZoneName  string // Cloud DNS managed zone name
	DNSName          string // DNS zone name (e.g., "network-name.internal.")
	FirewallRuleName string // VPC firewall rule name (placeholder — no compute client)
}

// VolumeState tracks volume state.
type VolumeState struct {
	BucketPath string // GCS bucket path (placeholder)
}
