package azurecommon

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
)

// ResolveAzureImageURI normalizes a Docker image reference for Azure Container Apps
// and Azure Functions.
//
// If the image is already an ACR URI (*.azurecr.io/*), it is returned as-is.
// If an acrName is provided, Docker Hub images are rewritten to pull from ACR.
// Otherwise, the reference is returned unchanged — ACA pulls Docker Hub refs
// directly.
//
// Examples (no acrName):
//
//	"alpine:latest"                → "alpine:latest" (unchanged)
//	"myorg/app:v1"                 → "myorg/app:v1" (unchanged)
//	"myacr.azurecr.io/img:tag"    → "myacr.azurecr.io/img:tag" (unchanged)
//
// Examples (acrName="myacr"):
//
//	"alpine:latest"                → "myacr.azurecr.io/library/alpine:latest"
//	"myorg/app:v1"                 → "myacr.azurecr.io/myorg/app:v1"
//	"myacr.azurecr.io/img:tag"    → "myacr.azurecr.io/img:tag" (unchanged)
func ResolveAzureImageURI(ref, acrName string) string {
	// Already an ACR URI — use as-is
	if strings.Contains(ref, ".azurecr.io/") {
		return ref
	}

	// Parse the Docker reference
	registry, repo, tag := parseDockerRef(ref)

	// Only handle Docker Hub images
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Docker Hub library images: "alpine" → "library/alpine"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	default:
		// Non-Docker Hub registries — return as-is
		return ref
	}

	// If ACR name is provided, rewrite to ACR URI
	if acrName != "" {
		// Strip any trailing .azurecr.io if someone passed the full hostname
		acrName = strings.TrimSuffix(acrName, ".azurecr.io")
		return acrName + ".azurecr.io/" + repo + ":" + tag
	}

	// No ACR — pass ref through unchanged; ACA pulls Docker Hub directly.
	return ref
}

// ResolveAzureImageURIWithCache rewrites a Docker image reference through
// an Azure Container Registry pull-through cache when a matching cache
// rule exists. This parallels AWS ECR pull-through + GCP
// Artifact Registry.
//
// Lookup flow:
//  1. If `ref` is already an ACR URI, return unchanged.
//  2. Normalize `ref` into a canonical upstream form
//     (`docker.io/<repo>:<tag>` for Docker Hub, else `<registry>/<repo>:<tag>`).
//  3. List cache rules on the registry; pick the one whose
//     `sourceRepository` is a prefix of the upstream form.
//  4. Rewrite to `<acrName>.azurecr.io/<targetRepository>:<tag>`, with
//     the repo tail matching the rule's target mapping.
//
// If no cache rule matches, `ref` is returned unchanged (ACA pulls
// Docker Hub refs directly; no docker.io fallback is added).
//
// The rulesClient is expected to point at the control-plane registry
// (real ACR live; the simulator's cacheRules sub-resource offline).
// The resourceGroup + registryName identify which registry to query.
func ResolveAzureImageURIWithCache(
	ctx context.Context,
	rulesClient *armcontainerregistry.CacheRulesClient,
	resourceGroup, registryName, ref string,
) (string, error) {
	// Already an ACR URI — use as-is.
	if strings.Contains(ref, ".azurecr.io/") {
		return ref, nil
	}
	// If no cache client or registry configured, pass through unchanged.
	if rulesClient == nil || registryName == "" {
		return ref, nil
	}

	registry, repo, tag := parseDockerRef(ref)
	var upstream string
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
		upstream = "docker.io/" + repo
	default:
		upstream = registry + "/" + repo
	}

	pager := rulesClient.NewListPager(resourceGroup, registryName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, rule := range page.Value {
			if rule == nil || rule.Properties == nil {
				continue
			}
			src := deref(rule.Properties.SourceRepository)
			tgt := deref(rule.Properties.TargetRepository)
			if src == "" || tgt == "" {
				continue
			}
			srcPrefix, isPrefix := trimWildcardSuffix(src)
			var tailRepo string
			switch {
			case upstream == src:
				// Exact single-repo match.
				tailRepo = tgt
			case isPrefix && strings.HasPrefix(upstream, srcPrefix):
				// Prefix (wildcard) rule: keep the sub-path after the prefix.
				tail := strings.TrimPrefix(upstream, srcPrefix)
				tgtPrefix, _ := trimWildcardSuffix(tgt)
				tailRepo = tgtPrefix + tail
			default:
				continue
			}
			acrName := strings.TrimSuffix(registryName, ".azurecr.io")
			return acrName + ".azurecr.io/" + tailRepo + ":" + tag, nil
		}
	}

	// No cache rule matched — pass through unchanged.
	return ref, nil
}

// trimWildcardSuffix strips a trailing `*` or `/*` from a cache-rule
// source/target repository, returning the prefix and a flag indicating
// whether the original was a wildcard pattern. Matches ACR's
// documented pattern syntax (e.g. `docker.io/library/*`).
func trimWildcardSuffix(s string) (string, bool) {
	if strings.HasSuffix(s, "/*") {
		return strings.TrimSuffix(s, "*"), true
	}
	if strings.HasSuffix(s, "*") {
		return strings.TrimSuffix(s, "*"), true
	}
	return s, false
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// parseDockerRef splits a Docker image reference into registry, repo, and tag.
// "nginx:alpine" → ("", "nginx", "alpine")
// "docker.io/library/alpine:3.18" → ("docker.io", "library/alpine", "3.18")
func parseDockerRef(ref string) (registry, repo, tag string) {
	tag = "latest"

	// Check for explicit registry (contains . or :port before first /)
	if i := strings.IndexByte(ref, '/'); i > 0 {
		prefix := ref[:i]
		if strings.ContainsAny(prefix, ".:") {
			registry = prefix
			ref = ref[i+1:]
		}
	}

	// Split repo:tag
	if i := strings.LastIndexByte(ref, ':'); i > 0 {
		repo = ref[:i]
		tag = ref[i+1:]
	} else {
		repo = ref
	}
	return
}
