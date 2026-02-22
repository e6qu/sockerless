package azf

// AZFState maps sockerless container IDs to Azure Functions resources.
type AZFState struct {
	FunctionAppName string
	ResourceID      string
	FunctionURL     string
	AgentToken      string
}
