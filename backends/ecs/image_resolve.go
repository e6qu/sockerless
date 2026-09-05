package ecs

import (
	"context"
	"fmt"
	"strings"

	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// resolveImageURI converts a Docker image reference to a registry that
// Fargate can pull from. The routing rules:
//
//  1. Already-ECR URIs (`<account>.dkr.ecr.<region>.amazonaws.com/...`)
//     pass through unchanged.
//  2. Already-public-gallery URIs (`public.ecr.aws/...`) pass through —
//     pullable by Fargate with no authentication.
//  3. Docker Hub library refs (`alpine`, `node:20`, `nginx:alpine` —
//     i.e. the official-image namespace) get rewritten to the AWS
//     Public Gallery Docker mirror at
//     `public.ecr.aws/docker/library/<name>:<tag>`. AWS hosts an
//     official mirror; no credentials needed.
//  4. Other public registries (`ghcr.io/...`, `quay.io/...`, etc.) get
//     routed through an ECR pull-through cache rule that points at the
//     upstream — none of these require credentials when caching public
//     images. Fargate then pulls the cached copy from ECR.
//  5. Docker Hub user/org refs (`myorg/myapp`) are rejected with a clear
//     error because (a) AWS Public Gallery only mirrors `library/` and
//     (b) Docker Hub user-image pull-through needs Docker Hub PAT-backed
//     auth which the project's no-credentials-on-disk discipline avoids
//     by design. Operators who need such an image should `docker push`
//     it to their ECR repo first.
func (s *Server) resolveImageURI(ctx context.Context, ref string) (string, error) {
	if awscommon.IsECRImageURI(ref) {
		return ref, nil
	}
	if strings.HasPrefix(ref, "public.ecr.aws/") {
		return ref, nil
	}

	// Digest-only refs (`sha256:...` with no preceding name) — try to
	// resolve via the local image Store. Sockerless's `docker pull` /
	// `docker tag` records each image with its tags + digest, so a
	// caller (e.g. gitlab-runner's volume-permission helper) that
	// references an image by digest after a previous pull can be
	// resolved back to the canonical name+tag we originally fetched.
	// If the digest isn't in the Store, surface a clear error rather
	// than misrouting through `public.ecr.aws/docker/library/sha256:...`
	// (which 404s and triggers a 7×-retry pull failure on Fargate).
	if strings.HasPrefix(ref, "sha256:") && !strings.ContainsAny(ref, "/@") {
		if img, ok := s.Store.ResolveImage(ref); ok && len(img.RepoTags) > 0 {
			canonical := img.RepoTags[0]
			s.Logger.Debug().Str("digest", ref).Str("canonical", canonical).
				Msg("resolved digest-only ref via image Store")
			return s.resolveImageURI(ctx, canonical)
		}
		return ref, fmt.Errorf(
			"digest-only image reference %q can't be resolved on ECS — Fargate pulls fresh per task and sockerless's image Store has no record of this digest. Either reference the image by name+digest (`name@%s`) or pull it first via `docker pull <name>`",
			ref, ref,
		)
	}

	// Locally-built images (e.g. `sockerless-eval-arithmetic:test`,
	// integration test images, runner-image dev iterations) live only
	// in the local image Store. Don't reroute them through Public
	// Gallery — the simulator's `ResolveLocalImage` only knows how
	// to strip ECR pull-through cache URIs back to local form, so a
	// `public.ecr.aws/docker/library/<local-name>` reference would
	// fail to resolve and the container would either pull a wrong
	// image or fail at task launch. Using the ref as-is keeps local
	// containers running off the local image directly.
	if _, ok := s.Store.ResolveImage(ref); ok {
		s.Logger.Debug().Str("ref", ref).Msg("image found in local Store; no registry rewrite needed")
		return ref, nil
	}

	registry, repo, tag := core.SplitDockerRef(ref)

	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Docker Hub. Library images are mirrored on AWS Public Gallery
		// at `public.ecr.aws/docker/library/<name>`; user/org images
		// aren't.
		if strings.Contains(repo, "/") {
			return ref, fmt.Errorf("docker hub user/org image %q is not on AWS Public Gallery; push it to your ECR repository first (sockerless avoids Docker Hub PAT credentials by design — use `docker push <ecr-uri>` to host the image yourself, or reference its public.ecr.aws equivalent if one exists)", ref)
		}
		mirrored := fmt.Sprintf("public.ecr.aws/docker/library/%s:%s", repo, tag)
		s.Logger.Debug().Str("original", ref).Str("public", mirrored).Msg("resolved Docker Hub library ref via AWS Public Gallery")
		return mirrored, nil
	}

	// Other registries (ghcr.io, quay.io, k8s.gcr.io, registry.k8s.io,
	// etc.). Route through an ECR pull-through cache rule. Unauthenticated
	// upstreams (which is most public-image registries) don't need any
	// CredentialArn — the rule just needs to exist.
	cachePrefix := awscommon.PullThroughCachePrefix(registry)
	accountID := awscommon.ExtractAccountID(s.config.ExecutionRoleARN)
	if accountID == "" {
		return ref, fmt.Errorf("cannot determine AWS account ID from ExecutionRoleARN %q", s.config.ExecutionRoleARN)
	}
	cache := awscommon.ECRPullThroughCache{Client: s.aws.ECR, Logger: s.Logger}
	if err := cache.Ensure(ctx, cachePrefix, registry, awscommon.UpstreamRegistryFor(registry)); err != nil {
		return ref, fmt.Errorf("ECR pull-through cache setup for %q: %w", cachePrefix, err)
	}

	ecrURI := awscommon.PullThroughCacheURI(accountID, s.config.Region, cachePrefix, repo, tag)
	s.Logger.Debug().Str("original", ref).Str("ecr", ecrURI).Msg("resolved image to ECR pull-through cache URI")
	return ecrURI, nil
}
