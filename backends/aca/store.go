package aca

// ACAState maps sockerless container IDs to Azure resource names.
type ACAState struct {
	JobName       string // ACA Job name
	ExecutionName string // Execution name
	ResourceGroup string // Resource group
	AgentAddress  string // "ip:port" of the agent
	AgentToken    string // Bearer token for agent auth
}

// NetworkState tracks virtual network state.
type NetworkState struct {
	// Virtual — no real VNet changes
}

// VolumeState tracks volume state.
type VolumeState struct {
	ShareName string // Azure Files share (placeholder)
}
