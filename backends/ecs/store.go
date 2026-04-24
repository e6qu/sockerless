package ecs

// ECSState maps sockerless container IDs to AWS resource ARNs and metadata.
type ECSState struct {
	TaskARN          string   // ECS task ARN
	TaskDefARN       string   // Task definition ARN
	ClusterARN       string   // Cluster ARN
	SecurityGroupIDs []string // Security groups from network associations (multiple networks)
	ServiceID        string   // Cloud Map service ID for service discovery
	RestartCount     int      // Next value for sockerless-restart-count tag on RunTask
}

// NetworkState maps sockerless network IDs to AWS resources.
type NetworkState struct {
	SecurityGroupID string
	NamespaceID     string // Cloud Map namespace
}
