package gcp_sdk_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	secretmanager "google.golang.org/api/secretmanager/v1"
)

func secretManagerService(t *testing.T) *secretmanager.Service {
	t.Helper()
	svc, err := secretmanager.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func TestSecretManagerSecretLifecycleSDK(t *testing.T) {
	svc := secretManagerService(t)
	parent := "projects/sdk-secret-project"
	secretID := "sdk-lifecycle-secret"
	secretName := parent + "/secrets/" + secretID

	created, err := svc.Projects.Secrets.Create(parent, &secretmanager.Secret{
		Labels: map[string]string{"env": "dev"},
	}).SecretId(secretID).Do()
	require.NoError(t, err)
	require.Equal(t, secretName, created.Name)
	require.Equal(t, map[string]string{"env": "dev"}, created.Labels)

	v1, err := svc.Projects.Secrets.AddVersion(secretName, &secretmanager.AddSecretVersionRequest{
		Payload: &secretmanager.SecretPayload{
			Data: base64.StdEncoding.EncodeToString([]byte("first")),
		},
	}).Do()
	require.NoError(t, err)
	require.Equal(t, secretName+"/versions/1", v1.Name)

	v2, err := svc.Projects.Secrets.AddVersion(secretName, &secretmanager.AddSecretVersionRequest{
		Payload: &secretmanager.SecretPayload{
			Data: base64.StdEncoding.EncodeToString([]byte("second")),
		},
	}).Do()
	require.NoError(t, err)
	require.Equal(t, secretName+"/versions/2", v2.Name)

	latest, err := svc.Projects.Secrets.Versions.Access(secretName + "/versions/latest").Do()
	require.NoError(t, err)
	require.Equal(t, secretName+"/versions/2", latest.Name)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("second")), latest.Payload.Data)

	versions, err := svc.Projects.Secrets.Versions.List(secretName).Do()
	require.NoError(t, err)
	require.Equal(t, int64(2), versions.TotalSize)
	require.Len(t, versions.Versions, 2)
	require.Equal(t, secretName+"/versions/2", versions.Versions[0].Name)
	require.Equal(t, secretName+"/versions/1", versions.Versions[1].Name)

	patched, err := svc.Projects.Secrets.Patch(secretName, &secretmanager.Secret{
		Labels: map[string]string{"env": "prod", "owner": "sdk"},
	}).UpdateMask("labels").Do()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"env": "prod", "owner": "sdk"}, patched.Labels)

	_, err = svc.Projects.Secrets.Delete(secretName).Do()
	require.NoError(t, err)

	_, err = svc.Projects.Secrets.Get(secretName).Do()
	require.Error(t, err)
	var apiErr *googleapi.Error
	require.True(t, errors.As(err, &apiErr), "expected googleapi.Error, got %T: %v", err, err)
	require.Equal(t, 404, apiErr.Code)
}

func TestSecretManager_ListPagination(t *testing.T) {
	svc := secretManagerService(t)
	proj := "pagination-sm-project"

	// Create 3 secrets.
	names := []string{
		"projects/" + proj + "/secrets/alpha",
		"projects/" + proj + "/secrets/beta",
		"projects/" + proj + "/secrets/gamma",
	}
	for _, n := range names {
		_, err := svc.Projects.Secrets.Create("projects/"+proj, &secretmanager.Secret{
			Replication: &secretmanager.Replication{Automatic: &secretmanager.Automatic{}},
		}).SecretId(n[len("projects/"+proj+"/secrets/"):]).Do()
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		for _, n := range names {
			_, _ = svc.Projects.Secrets.Delete(n).Do()
		}
	})

	// Page through with pageSize=1 and collect all secrets.
	var got []string
	pageToken := ""
	for {
		call := svc.Projects.Secrets.List("projects/" + proj).PageSize(1)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		require.NoError(t, err)
		require.Len(t, resp.Secrets, 1, "each page must have exactly 1 secret with pageSize=1")
		got = append(got, resp.Secrets[0].Name)
		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}
	require.Len(t, got, 3, "all 3 secrets must be returned across pages")
	// All 3 names must appear (order may vary by store iteration).
	for _, n := range names {
		require.Contains(t, got, n)
	}
}
