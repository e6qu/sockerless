package aca

// ACAState maps sockerless container IDs to Azure resource names.
// Jobs and Apps are mutually exclusive at the backend level (chosen by
// Config.UseApp), but kept in the same struct so the cache layer
// doesn't need to branch on execution model.
type ACAState struct {
	JobName       string // ACA Job name (UseApp=false path)
	ExecutionName string // Execution name (UseApp=false path)
	ResourceGroup string // Resource group
	AppName       string // ACA ContainerApp name (UseApp=true path)
}

// NetworkState tracks cloud networking state for a Docker network.
type NetworkState struct {
	NSGName      string   // Network Security Group name
	NSGRuleNames []string // NSG rule names for this network
	DNSZoneName  string   // Azure Private DNS zone backing this network
}
