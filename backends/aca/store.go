package aca

// ACAState maps sockerless container IDs to Azure resource names.
type ACAState struct {
	JobName       string // ACA Job name
	ExecutionName string // Execution name
	ResourceGroup string // Resource group
}

// NetworkState tracks cloud networking state for a Docker network.
type NetworkState struct {
	NSGName      string   // Network Security Group name
	NSGRuleNames []string // NSG rule names for this network
	DNSZoneName  string   // Azure Private DNS zone backing this network (BUG-702)
}

// VolumeState tracks volume state.
type VolumeState struct {
	ShareName string // Azure Files share (placeholder)
}
