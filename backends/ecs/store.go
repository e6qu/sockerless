package ecs

// ECSState maps sockerless container IDs to AWS resource ARNs and metadata.
type ECSState struct {
	TaskARN          string   // ECS task ARN
	TaskDefARN       string   // Task definition ARN
	ClusterARN       string   // Cluster ARN
	SecurityGroupIDs []string // Security groups from network associations (multiple networks)
	ServiceID        string   // Cloud Map service ID for service discovery
	RestartCount     int      // Next value for sockerless-restart-count tag on RunTask
	// OpenStdin: container was created with OpenStdin && AttachStdin
	// (gitlab-runner / `docker run -i` pattern). The attach driver
	// uses this to decide whether to wire a per-cycle stdin pipe.
	// Persisted in ECSState (rather than Container.Config) because
	// CloudState's synthesized Container.Config doesn't carry stdin
	// flags, and gitlab-runner restarts the same container ID across
	// script steps — each cycle's attach must recognise the stdin
	// pattern even after PendingCreates is dropped.
	OpenStdin bool
}

// NetworkState maps sockerless network IDs to AWS resources.
type NetworkState struct {
	SecurityGroupID string
	NamespaceID     string // Cloud Map namespace
}
