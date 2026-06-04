# bleephub — Releases

Surface: `bleephub/gh_releases.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/releases>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Releases

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create release | `POST /api/v3/repos/{owner}/{repo}/releases` | ✓ `gh_releases.go::handleCreateRelease` | ✓ `gh_releases_test.go` | |
| List releases | `GET /api/v3/repos/{owner}/{repo}/releases` | ✓ `handleListReleases` | ✓ same | |
| Get latest release | `GET /api/v3/repos/{owner}/{repo}/releases/latest` | ✓ `handleGetLatestRelease` | ✓ same | |
| Generate release notes | `POST /api/v3/repos/{owner}/{repo}/releases/generate-notes` | ✓ `handleGenerateReleaseNotes` | ✓ same | |
| Get release | `GET /api/v3/repos/{owner}/{repo}/releases/{release_id}` | ✓ `handleGetRelease` | ✓ same | |
| Update release | `PATCH /api/v3/repos/{owner}/{repo}/releases/{release_id}` | ✓ `handleUpdateRelease` | ✓ same | |
| Delete release | `DELETE /api/v3/repos/{owner}/{repo}/releases/{release_id}` | ✓ `handleDeleteRelease` | ✓ same | |

## Release assets

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List release assets | `GET /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}` | ✓ `handleReleaseAssetsDispatch` | ✓ `gh_releases_test.go` | Routes GET on assets sub-resource. |
| Upload / update asset | `POST /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}` | ✓ `handleReleaseAssetUploadDispatch` | ✓ same | |
| Delete asset | `DELETE /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}/{p3}` | ✓ `handleReleaseAssetDeleteDispatch` | ✓ same | |
