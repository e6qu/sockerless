package cloudrun

// CloudRunState maps sockerless container IDs to GCP resource names.
type CloudRunState struct {
	JobName       string // Cloud Run Job name
	ExecutionName string // Execution name
	AgentAddress  string // "ip:port" of the agent
	AgentToken    string // Bearer token for agent auth
}

// NetworkState tracks virtual network state.
type NetworkState struct {
	// Virtual — no real Cloud DNS zones
}

// VolumeState tracks volume state.
type VolumeState struct {
	BucketPath string // GCS bucket path (placeholder)
}
