// Per-backend translator: cloud-agnostic core.BackingSpec → Lambda's
// FileSystemConfig + companion artifacts. Phase 91c puts the framework
// dispatch scaffolding in place; the deeper migration of
// `fileSystemConfigsForBinds` (which encodes Lambda-specific
// constraints — at-most-one FSC per function, /mnt/[A-Za-z0-9_.\-]+
// path constraint, sub-path collapse — and predates the BackingSpec
// framework) is bookmarked.
//
// Today's caller path: ContainerCreate → fileSystemConfigsForBinds
// (volumes.go) → emits FileSystemConfig directly.
//
// Future caller path: ContainerCreate → fileSystemConfigsForBinds →
// per-bind storageBackings.Resolve → translateBackingSpecToLambda →
// (collapse to one FSC per Lambda's constraints) → emit
// FileSystemConfig.
//
// This file ships the per-bind translator now so the rejection arms
// (BackingMemory etc.) surface before the migration lands. When
// fileSystemConfigsForBinds is eventually rewritten on top of the
// framework, the rejection messaging is already in place.

package lambda

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	core "github.com/sockerless/backend-core"
)

// translateBackingSpecToLambda materialises a cloud-agnostic
// BackingSpec into a Lambda FileSystemConfig. Pure-function shape
// matches the ECS / ACA / AZF translators; tests can drive it
// without spinning up a Lambda backend.
//
// Lambda accepts at most one FileSystemConfig per function — that
// constraint is honored by fileSystemConfigsForBinds in volumes.go,
// not here. This translator handles the per-spec→FSC conversion;
// the caller aggregates and rejects multi-AP cases.
func translateBackingSpecToLambda(spec core.BackingSpec) (*lambdatypes.FileSystemConfig, error) {
	switch spec.Kind {
	case core.BackingEFSEphemeral:
		if spec.EFSEphemeral == nil {
			return nil, fmt.Errorf("lambda translator: efs-ephemeral spec missing payload")
		}
		// FileSystemConfig wants the access-point ARN, not the ID. The
		// caller resolves ARN before constructing the spec; the
		// translator just packages the result.
		if spec.EFSEphemeral.AccessPointID == "" {
			return nil, fmt.Errorf("lambda translator: efs-ephemeral spec missing AccessPointID (resolve via EFSManager.AccessPointForVolume first)")
		}
		return &lambdatypes.FileSystemConfig{
			Arn:            aws.String(spec.EFSEphemeral.AccessPointID),
			LocalMountPath: aws.String(LambdaSharedMountPath),
		}, nil

	case core.BackingMemory:
		// Lambda has no tmpfs / RAM-backed mount primitive at the
		// FileSystemConfig surface. Per-invocation `/tmp` (configurable
		// 512 MB–10 GB ephemeral storage) is the closest analogue but
		// isn't a Docker-style mount — sockerless can't translate
		// `Backing: memory` to a Lambda volume primitive without lying
		// about the runtime semantics.
		return nil, fmt.Errorf(
			"lambda translator: backing %q not supported on Lambda — "+
				"Lambda has no tmpfs volume primitive at the FileSystemConfig layer. "+
				"Use per-invocation /tmp scratch space inside the function (configure size via "+
				"SOCKERLESS_LAMBDA_EPHEMERAL_STORAGE_SIZE up to 10240 MB) or pick "+
				"Backing: efs-ephemeral for cross-invocation durable storage",
			spec.Kind)
	}
	return nil, fmt.Errorf("lambda translator: backing %q not supported on Lambda", spec.Kind)
}
