package gcf

// GCFState maps sockerless container IDs to Cloud Run Functions resources.
type GCFState struct {
	FunctionName string
	FunctionURL  string
	LogResource  string
	AgentToken   string
}
