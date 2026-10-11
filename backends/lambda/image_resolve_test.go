package lambda

import (
	"context"
	"testing"
)

// TestImageReferencesFollowTheECRCoordinate: with the Amazon ECR registry
// endpoint coordinate set, an image reference already in the account's
// registry and the overlay repository both carry the coordinate's host, so
// the overlay build pushes, and the function pulls, where the registry is
// reached; without it they name the real registry.
func TestImageReferencesFollowTheECRCoordinate(t *testing.T) {
	const real = "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	for _, tc := range []struct {
		name, endpoint, wantHost string
	}{
		{"real registry", "", real},
		{"relocated registry", "http://127.0.0.1:4566", "127.0.0.1:4566"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{config: Config{
				Region:              "us-east-1",
				RoleARN:             "arn:aws:iam::123456789012:role/sockerless",
				ECRRegistryEndpoint: tc.endpoint,
			}}
			got, err := s.resolveImageURI(context.Background(), real+"/app:v1")
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.wantHost + "/app:v1"; got != want {
				t.Errorf("resolveImageURI = %q, want %q", got, want)
			}

			repo, err := s.overlayECRRepo()
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.wantHost + "/sockerless-live-lambda"; repo != want {
				t.Errorf("default overlayECRRepo = %q, want %q", repo, want)
			}

			s.config.OverlayECRRepo = real + "/sockerless-simulator-lambda"
			repo, err = s.overlayECRRepo()
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.wantHost + "/sockerless-simulator-lambda"; repo != want {
				t.Errorf("configured overlayECRRepo = %q, want %q", repo, want)
			}
		})
	}
}
