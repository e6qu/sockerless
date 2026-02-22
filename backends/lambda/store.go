package lambda

// LambdaState maps sockerless container IDs to AWS Lambda resources.
type LambdaState struct {
	FunctionName  string
	FunctionARN   string
	LogStreamName string
	AgentToken    string
}
