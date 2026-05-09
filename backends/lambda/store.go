package lambda

// LambdaState maps sockerless container IDs to AWS Lambda resources.
type LambdaState struct {
	FunctionName  string
	FunctionARN   string
	LogStreamName string
	// OpenStdin: container was created with OpenStdin && AttachStdin
	// (gitlab-runner / `docker run -i` pattern). The attach driver
	// uses this to wire a per-cycle stdin pipe; ContainerStart
	// defers Lambda Invoke until stdin EOF and bakes the buffered
	// bytes into the Invoke Payload — the bootstrap's
	// `runUserInvocation` already pipes Payload to the user
	// entrypoint as stdin, so `Cmd=[sh]` + Payload=script naturally
	// runs the buffered script.
	OpenStdin bool
	// ServiceID is the per-hostname Cloud Map service ID this
	// invocation registered against (when service-mesh
	// NetworkDiscovery is enabled). Empty for nat-gateway-only and
	// host-aliases. Set via awscommon.CloudMapDiscovery's
	// SetContainerServiceID callback at register time.
	ServiceID string
}

// NetworkState maps sockerless network IDs to AWS resources. Lambda
// uses Cloud Map private DNS namespaces tied to the configured VPC for
// service-mesh peer discovery; the per-network namespace is created
// at NetworkCreate time.
type NetworkState struct {
	NamespaceID string
}
