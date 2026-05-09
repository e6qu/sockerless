// Access driver mechanism categories.
//
// Sibling to NetworkDiscoveryKind and DNSMechanism. Models two coupled
// concerns: (1) the cloud-native principal a workload runs as, and
// (2) the per-call credential a caller mints to invoke the workload's
// HTTP endpoint. Each backend declares one mechanism; the driver
// abstracts the caller-side signer so HTTP invocation paths stay
// uniform across clouds.

package api

// AccessMechanism names a cloud-side ingress-auth mechanism.
type AccessMechanism string

const (
	// AccessMechanismIAMRole binds an AWS IAM role to the workload
	// (ECS TaskRole, Lambda execution role) and relies on SigV4 at the
	// AWS SDK layer for caller-side signing. Non-SDK callers use the
	// default HTTP client; SDK callers sign automatically.
	AccessMechanismIAMRole AccessMechanism = "iam-role"

	// AccessMechanismIDToken binds a GCP service account to the
	// workload (Cloud Run service-account, Cloud Functions
	// service-account) and relies on a Google ID token JWT (audience
	// = workload URL) on every caller-side invocation. Verified by
	// the platform's built-in IAM check.
	AccessMechanismIDToken AccessMechanism = "id-token"

	// AccessMechanismMTLS uses mutual TLS at the transport layer.
	// Workload presents a TLS server cert; caller presents a TLS
	// client cert. No per-request header credentials.
	AccessMechanismMTLS AccessMechanism = "mTLS"

	// AccessMechanismNoneInternal disables ingress auth entirely.
	// Used when the workload is reachable only over a private VPC
	// (ACA managed environment, AZF on a private endpoint) — the
	// network layer enforces access control.
	AccessMechanismNoneInternal AccessMechanism = "none-internal"
)

// IsValid reports whether m is one of the documented mechanisms.
func (m AccessMechanism) IsValid() bool {
	switch m {
	case AccessMechanismIAMRole,
		AccessMechanismIDToken,
		AccessMechanismMTLS,
		AccessMechanismNoneInternal:
		return true
	}
	return false
}

// String makes AccessMechanism satisfy fmt.Stringer.
func (m AccessMechanism) String() string { return string(m) }

// AllAccessMechanisms is the closed set of valid mechanisms.
var AllAccessMechanisms = []AccessMechanism{
	AccessMechanismIAMRole,
	AccessMechanismIDToken,
	AccessMechanismMTLS,
	AccessMechanismNoneInternal,
}
