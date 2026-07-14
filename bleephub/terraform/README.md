# Bleephub Amazon Elastic Container Service on AWS Fargate Module

This Terraform module provisions Bleephub in its own AWS network and account
scope. It creates the private Amazon Elastic Container Service on AWS Fargate
services, native dqlite storage, Amazon Simple Storage Service git/object
storage, Amazon API Gateway wake routing, private administrator origin, SSH Git
gateway, Route 53 records, certificate, fck-nat, and Amazon Simple Storage
Service gateway endpoint.

The module deliberately contains no backend or environment-specific values.
Use it through Terragrunt from the private `e6qu/infra` repository. The
production environment is `bleephub`, with `bleephub.e6qu.dev` as the public
origin and `admin.bleephub.e6qu.dev` as the administrator origin.

## Required inputs

- `name` — stable AWS resource prefix.
- `hosted_zone_id` — Route 53 hosted zone containing `bleephub.e6qu.dev`.
- `domain_name` — `bleephub.e6qu.dev`.
- `container_image` — immutable Bleephub release image coordinate.
- `admin_token` — initial administrator secret; provide it through the
  Terragrunt environment rather than committing it.
- `wake_listener_zip_path` — pre-built Linux Amazon Lambda wake-listener ZIP.

`github_oauth_client_id` and `github_oauth_client_secret_arn` enable the
registered GitHub OAuth App. The secret ARN references an existing AWS Secrets
Manager secret so Terraform never receives the OAuth client secret value.

## Outputs

The module returns the public Bleephub URL, administrator URL, SSH host,
service and Amazon API Gateway identifiers, durable object-store names, and
the AWS Secrets Manager ARN holding the administrator token.

## Validation

The module's Amazon Web Services simulator apply/destroy test lives in `test/`.
Build the wake-listener artifact with:

```bash
scripts/build-bleephub-wake.sh
```
