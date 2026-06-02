package main

import (
	"context"
	"fmt"
	"io"

	dockerimage "github.com/docker/docker/api/types/image"
	sim "github.com/sockerless/simulator"
)

func localImagePlatform(ctx context.Context, imageRef string) (string, error) {
	cli := sim.DockerClient()
	if cli == nil {
		return "", fmt.Errorf("docker client not initialized")
	}
	inspect, _, err := cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		rc, pullErr := cli.ImagePull(ctx, imageRef, dockerimage.PullOptions{})
		if pullErr != nil {
			return "", fmt.Errorf("inspect image %q platform: %w; pull image: %w", imageRef, err, pullErr)
		}
		if _, copyErr := io.Copy(io.Discard, rc); copyErr != nil {
			_ = rc.Close()
			return "", fmt.Errorf("pull image %q: %w", imageRef, copyErr)
		}
		if closeErr := rc.Close(); closeErr != nil {
			return "", fmt.Errorf("close image pull stream %q: %w", imageRef, closeErr)
		}
		inspect, _, err = cli.ImageInspectWithRaw(ctx, imageRef)
		if err != nil {
			return "", fmt.Errorf("inspect pulled image %q platform: %w", imageRef, err)
		}
	}
	if inspect.Os == "" || inspect.Architecture == "" {
		return "", fmt.Errorf("inspect image %q platform: missing os/architecture", imageRef)
	}
	return inspect.Os + "/" + inspect.Architecture, nil
}

func lambdaDockerPlatform(architectures []string) (string, error) {
	if len(architectures) == 0 {
		return "linux/amd64", nil
	}
	switch architectures[0] {
	case "x86_64":
		return "linux/amd64", nil
	case "arm64":
		return "linux/arm64", nil
	default:
		return "", fmt.Errorf("unsupported Lambda architecture %q", architectures[0])
	}
}
