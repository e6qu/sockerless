# Image Scanning & Security Specification

Vulnerability scanning, image signing, and supply chain security per cloud.

## Scanning Services

| Cloud | Service | API | Trigger | Results |
|-------|---------|-----|---------|---------|
| AWS | ECR Image Scanning (Inspector) | `ecr.StartImageScan` / `ecr.DescribeImageScanFindings` | On push or manual | CVE list with severity |
| GCP | Artifact Analysis | `containeranalysis.ListOccurrences` | Automatic on push | CVE list with severity + fix versions |
| Azure | Defender for Containers | Microsoft Defender API | Automatic on push | CVE list with severity + recommendations |

### Interface

```go
// ImageScanner scans container images for vulnerabilities.
type ImageScanner interface {
    // Scan triggers or retrieves scan results for an image.
    Scan(ctx context.Context, imageRef string) (*ScanResult, error)

    // Available returns true if scanning is configured.
    Available() bool
}

type ScanResult struct {
    ImageRef       string
    ScanStatus     string        // "COMPLETE", "IN_PROGRESS", "FAILED"
    Vulnerabilities []Vulnerability
    ScanTime       time.Time
}

type Vulnerability struct {
    ID          string  // CVE-2024-1234
    Package     string  // openssl
    Version     string  // 1.1.1
    Severity    string  // CRITICAL, HIGH, MEDIUM, LOW, INFORMATIONAL
    FixVersion  string  // 1.1.2 (empty if no fix)
    Description string
    URI         string  // Link to CVE detail
}
```

### AWS ECR Scanning

```go
// Trigger scan
ecr.StartImageScan({
    RepositoryName: repo,
    ImageId: { ImageTag: tag },
})

// Poll results
ecr.DescribeImageScanFindings({
    RepositoryName: repo,
    ImageId: { ImageTag: tag },
})
// → findings.ImageScanFindings.Findings[]
```

ECR supports two scan types:
- **Basic scanning**: Uses Clair (open source). Free.
- **Enhanced scanning**: Uses AWS Inspector. Per-scan cost. Richer results.

### GCP Artifact Analysis

Automatic on push when Container Analysis API is enabled. No explicit trigger needed.

```go
// List vulnerabilities
containeranalysis.ListOccurrences({
    Parent: "projects/{project}",
    Filter: 'resourceUrl="https://{registry}/{repo}@{digest}" AND kind="VULNERABILITY"',
})
// → occurrences[].vulnerability.{severity, packageIssue[]}
```

### Azure Defender for Containers

Automatic when Defender plan is enabled on the subscription. Results via Defender API or Security Center.

```go
// Query via Azure Resource Graph
resourcegraph.Resources({
    Query: 'securityresources | where type == "microsoft.security/assessments/subassessments" | where properties.id contains "{imageDigest}"'
})
```

## Image Signing

| Cloud | Service | Standard | Key Storage |
|-------|---------|----------|-------------|
| AWS | AWS Signer | Notation | AWS KMS |
| GCP | Binary Authorization | Cosign / Attestation | Cloud KMS |
| Azure | Notation (ORAS) | Notation | Azure Key Vault |

### Interface

```go
// ImageSigner signs and verifies container image signatures.
type ImageSigner interface {
    // Sign creates a signature for an image digest.
    Sign(ctx context.Context, imageRef string) (*SignResult, error)

    // Verify checks if an image has a valid signature.
    Verify(ctx context.Context, imageRef string) (*VerifyResult, error)

    // Available returns true if signing is configured.
    Available() bool
}

type SignResult struct {
    ImageRef   string
    Digest     string
    Signature  string // Base64 signature
    SignedAt   time.Time
    SigningKey string // Key identifier
}

type VerifyResult struct {
    Verified bool
    Signer   string // Identity of signer
    SignedAt time.Time
}
```

### AWS Signer

```go
// Sign
signer.StartSigningJob({
    Source: { S3: { BucketName, Key, Version } },
    Destination: { S3: { BucketName, Prefix } },
    ProfileName: "sockerless-signing-profile",
})

// Verify — done at deployment time via ECR image policy
```

### GCP Binary Authorization

```go
// Create attestation (sign)
containeranalysis.CreateOccurrence({
    Parent: "projects/{project}",
    Occurrence: {
        ResourceUri: "https://{registry}/{repo}@{digest}",
        NoteName: "projects/{project}/notes/{attestor}",
        Attestation: { Serialized: signedPayload },
    },
})

// Verify — done at deploy time via Binary Authorization policy on GKE/Cloud Run
```

### Azure Notation

```go
// Sign with Notation CLI (no SDK — shell out or use notation-go library)
// notation sign --key <keyvault-key> <registry>/<repo>@<digest>

// Verify
// notation verify <registry>/<repo>@<digest> --trust-policy <policy>
```

## Docker API Mapping

No Docker API endpoint exists for scan or sign. These are cloud-side operations triggered:

1. **On push** — scan automatically (ECR enhanced, Artifact Analysis, Defender)
2. **On build** — sign after successful build + push
3. **On pull** — verify signature before allowing container start (policy-based)

### Sockerless Integration Points

| Operation | When | What |
|-----------|------|------|
| `ImagePush` completion | After successful push | Trigger scan, wait for results |
| `ImageBuild` completion | After cloud build pushes | Trigger sign |
| `ContainerStart` | Before RunTask/CreateJob | Verify signature (if policy requires) |
| `docker scan` (CLI plugin) | User-initiated | Fetch scan results from cloud |

## Priority

| Feature | Priority | Reason |
|---------|----------|--------|
| ECR basic scanning | Medium | Free, automatic, useful for CI |
| Artifact Analysis | Medium | Automatic, useful for CI |
| Defender scanning | Medium | Automatic if enabled |
| Image signing | Low | Enforcement requires policy config |
| Signature verification | Low | Only needed when policy is set |
| SBOM generation | Low | OCI 1.1 artifact, emerging standard |
