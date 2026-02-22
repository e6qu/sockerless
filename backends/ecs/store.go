package ecs

// ECSState maps sockerless container IDs to AWS resource ARNs and metadata.
type ECSState struct {
	TaskARN      string // ECS task ARN
	TaskDefARN   string // Task definition ARN
	ClusterARN   string // Cluster ARN
	AgentAddress string // "ip:port" of the agent inside the task
	AgentToken   string // Bearer token for agent auth
}

// NetworkState maps sockerless network IDs to AWS resources.
type NetworkState struct {
	SecurityGroupID string
	NamespaceID     string // Cloud Map namespace
}

// VolumeState maps sockerless volume names to AWS resources.
type VolumeState struct {
	EFSFileSystemID string
	AccessPointID   string
}
