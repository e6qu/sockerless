# AWS CodeBuild

Surface: `simulators/aws/codebuild.go`.

Canonical reference: <https://docs.aws.amazon.com/codebuild/latest/APIReference/>

Protocol: AWS JSON 1.1 (`X-Amz-Target: CodeBuild_20161006.<Op>`).

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Projects

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| CreateProject | `CodeBuild_20161006.CreateProject` | ✓ `TestCodeBuild_ProjectCRUD_SDK` | ✓ `TestCodeBuild_ProjectCRUD_CLI` | ✓ `aws_codebuild_project` | Returns full project object. |
| BatchGetProjects | `CodeBuild_20161006.BatchGetProjects` | ✓ | ✓ | ✓ | Returns `projects` + `projectsNotFound` arrays. |
| ListProjects | `CodeBuild_20161006.ListProjects` | ✓ | ✓ | ✓ | Returns project name list with pagination. |
| UpdateProject | `CodeBuild_20161006.UpdateProject` | ✓ | ✗ | ✓ | |
| DeleteProject | `CodeBuild_20161006.DeleteProject` | ✓ | ✓ | ✓ | |
| TagResource | `CodeBuild_20161006.TagResource` | ✗ | ✗ | ✓ | SDK v2 does not expose as a separate client method; tags verified via `BatchGetProjects`. |
| UntagResource | `CodeBuild_20161006.UntagResource` | ✗ | ✗ | ✓ | |
| ListTagsForResource | `CodeBuild_20161006.ListTagsForResource` | ✗ | ✗ | ✓ | |

## Builds

| Operation | X-Amz-Target | SDK test | CLI test | TF resource | notes |
|---|---|---|---|---|---|
| StartBuild | `CodeBuild_20161006.StartBuild` | ✓ `TestCodeBuild_BuildLifecycle_SDK` | ✓ `TestCodeBuild_Build_CLI` | — | Completes immediately with `SUCCEEDED`. |
| BatchGetBuilds | `CodeBuild_20161006.BatchGetBuilds` | ✓ | ✓ | — | Returns `builds` + `buildsNotFound`. |
| ListBuildsForProject | `CodeBuild_20161006.ListBuildsForProject` | ✓ | ✓ | — | Paginated build ID list. |
| ListBuilds | `CodeBuild_20161006.ListBuilds` | ✗ | ✗ | — | All builds across all projects. |
