package awscommon

import (
	"fmt"
	"strings"

	core "github.com/sockerless/backend-core"
)

// The Amazon ECR registry endpoint coordinate.
//
// An Amazon ECR private registry is reached at
// `<account>.dkr.ecr.<region>.amazonaws.com`, and that host is both the network
// destination of every registry request and the host an image reference into
// the registry carries. An operator relocates the registry — a harness pointed
// at the simulator, a registry reached through an interface endpoint's own DNS
// name — by setting SOCKERLESS_AWS_ECR_ENDPOINT, the coordinate gcp-common
// honours as SOCKERLESS_GCP_AR_ENDPOINT and azure-common as
// SOCKERLESS_AZURE_ACR_ENDPOINT. A backend reads it once into its Config and
// hands that value to every place the registry is named or dialed: the image
// references a workload pulls, the overlay a build pushes, and the auth
// provider that recognises and dials the registry. The code is identical
// against cloud and simulator; only the coordinate differs.

// ECREndpointEnv is the environment variable that carries the coordinate.
const ECREndpointEnv = "SOCKERLESS_AWS_ECR_ENDPOINT"

// ECRRegistryHost returns the registry host image references into the
// account's private registry carry: the coordinate's host when one is set,
// else the real `<account>.dkr.ecr.<region>.amazonaws.com`.
func ECRRegistryHost(accountID, region, endpoint string) string {
	if host := core.RegistryCoordinateHost(endpoint); host != "" {
		return host
	}
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", accountID, region)
}

// IsECRRegistry reports whether `registry` — the host of an image reference —
// is an Amazon ECR private registry: the real `*.dkr.ecr.*.amazonaws.com`
// shape, or the relocated coordinate's host.
func IsECRRegistry(registry, endpoint string) bool {
	if strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.") {
		return true
	}
	host := core.RegistryCoordinateHost(endpoint)
	return host != "" && registry == host
}

// IsECRImageURI reports whether ref already names a repository in an Amazon
// ECR private registry, at its real host or at the coordinate.
func IsECRImageURI(ref, endpoint string) bool {
	registry, _, found := strings.Cut(ref, "/")
	return found && IsECRRegistry(registry, endpoint)
}

// RelocateECRReference returns ref with a real Amazon ECR registry host
// replaced by the coordinate's host, so a reference an operator or Terraform
// wrote against `<account>.dkr.ecr.<region>.amazonaws.com` reaches the
// registry where the coordinate puts it. Any other reference, or any
// reference when no coordinate is set, is returned unchanged.
func RelocateECRReference(ref, endpoint string) string {
	host := core.RegistryCoordinateHost(endpoint)
	if host == "" {
		return ref
	}
	registry, rest, found := strings.Cut(ref, "/")
	if !found || !strings.HasSuffix(registry, ".amazonaws.com") || !strings.Contains(registry, ".dkr.ecr.") {
		return ref
	}
	return host + "/" + rest
}
