# Simulator SDK / CLI / Terraform Coverage Matrix

This matrix is the maintained client-surface index for the simulator testing contract. Each row corresponds exactly to one canonical surface table in `specs/SIM_SURFACE_TABLES/`; `scripts/check-simulator-coverage-matrix.sh` fails CI if rows drift from that directory.

Legend:

- `direct` means a real official SDK, vendor CLI, or Terraform provider flow exercises the surface.
- `not applicable` means that client family does not expose that cloud surface in a meaningful way for the implemented simulator slice.
- `tracked #...` means a broader implementation issue owns that surface family.

| Surface | SDK | CLI | Terraform | Evidence |
|---|---|---|---|---|
| `aws-acm` | direct | direct | direct | `simulators/aws/sdk-tests/acm_test.go`; `simulators/aws/cli-tests/acm_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-amplify` | direct | direct | direct | `simulators/aws/sdk-tests/amplify_test.go`; `simulators/aws/cli-tests/amplify_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-apigateway` | direct | direct | direct | `simulators/aws/sdk-tests/apigateway_test.go`; `simulators/aws/sdk-tests/apigateway_method_response_test.go`; `simulators/aws/cli-tests/apigateway_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-apigatewayv2` | direct | direct | direct | `simulators/aws/sdk-tests/apigatewayv2_deployment_test.go`; `simulators/aws/cli-tests/apigateway_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-application-autoscaling` | direct | direct | direct | `simulators/aws/sdk-tests/application_autoscaling_test.go`; `simulators/aws/cli-tests/application_autoscaling_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-autoscaling` | direct | direct | direct | `simulators/aws/sdk-tests/autoscaling_cloudtrail_test.go`; `simulators/aws/cli-tests/autoscaling_cloudtrail_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-cloudmap` | direct | direct | direct | `simulators/aws/sdk-tests/cloudmap_test.go`; `simulators/aws/cli-tests/cloudmap_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-cloudtrail` | direct | direct | direct | `simulators/aws/sdk-tests/autoscaling_cloudtrail_test.go`; `simulators/aws/cli-tests/autoscaling_cloudtrail_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-cloudwatch` | direct | direct | direct | `simulators/aws/sdk-tests/cloudwatch_test.go`; `simulators/aws/cli-tests/cloudwatch_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-dynamodb` | direct | direct | direct | `simulators/aws/sdk-tests/dynamodb_test.go`; `simulators/aws/cli-tests/dynamodb_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-ec2` | direct | direct | direct | `simulators/aws/sdk-tests/ec2_test.go`; `simulators/aws/cli-tests/ec2_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-ecr` | direct | direct | direct | `simulators/aws/sdk-tests/ecr_test.go`; `simulators/aws/cli-tests/ecr_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-ecs` | direct | direct | direct | `simulators/aws/sdk-tests/ecs_test.go`; `simulators/aws/cli-tests/ecs_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-efs` | direct | direct | direct | `simulators/aws/sdk-tests/efs_test.go`; `simulators/aws/cli-tests/efs_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-elasticache` | direct | direct | direct | `simulators/aws/sdk-tests/rds_elasticache_test.go`; `simulators/aws/cli-tests/rds_elasticache_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-elbv2` | direct | direct | direct | `simulators/aws/sdk-tests/elbv2_test.go`; `simulators/aws/cli-tests/elbv2_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-eventbridge` | direct | direct | direct | `simulators/aws/sdk-tests/eventbridge_test.go`; `simulators/aws/cli-tests/eventbridge_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-iam` | direct | direct | direct | `simulators/aws/sdk-tests/iam_test.go`; `simulators/aws/cli-tests/iam_slr_oidc_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-kinesis` | direct | direct | direct | `simulators/aws/sdk-tests/kinesis_test.go`; `simulators/aws/cli-tests/kinesis_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-kms` | direct | direct | direct | `simulators/aws/sdk-tests/kms_test.go`; `simulators/aws/cli-tests/kms_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-lambda` | direct | direct | direct | `simulators/aws/sdk-tests/lambda_test.go`; `simulators/aws/cli-tests/lambda_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-rds` | direct | direct | direct | `simulators/aws/sdk-tests/rds_elasticache_test.go`; `simulators/aws/sdk-tests/rds_snapshot_test.go`; `simulators/aws/cli-tests/rds_elasticache_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-route53` | direct | direct | direct | `simulators/aws/sdk-tests/route53_test.go`; `simulators/aws/cli-tests/route53_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-s3` | direct | direct | direct | `simulators/aws/sdk-tests/s3_test.go`; `simulators/aws/cli-tests/s3_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-s3-bucket-subresources` | direct | direct | direct | `simulators/aws/sdk-tests/s3_bucket_subresources_test.go`; `simulators/aws/cli-tests/s3_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-scheduler` | direct | direct | direct | `simulators/aws/sdk-tests/scheduler_test.go`; `simulators/aws/cli-tests/scheduler_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-s3-multipart` | direct | direct | not applicable | `simulators/aws/sdk-tests/s3_list_parts_test.go`; `simulators/aws/cli-tests/s3_test.go` |
| `aws-secretsmanager` | direct | direct | direct | `simulators/aws/sdk-tests/secretsmanager_test.go`; `simulators/aws/cli-tests/secretsmanager_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-sns` | direct | direct | direct | `simulators/aws/sdk-tests/sns_sqs_ops_test.go`; `simulators/aws/cli-tests/sqs_sns_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-sqs` | direct | direct | direct | `simulators/aws/sdk-tests/sqs_test.go`; `simulators/aws/cli-tests/sqs_sns_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-ssm_parameters` | direct | direct | direct | `simulators/aws/sdk-tests/ssm_parameters_test.go`; `simulators/aws/cli-tests/ssm_parameters_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-sts` | direct | direct | direct | `simulators/aws/sdk-tests/sts_test.go`; `simulators/aws/cli-tests/sts_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-wafv2` | direct | direct | direct | `simulators/aws/sdk-tests/wafv2_test.go`; `simulators/aws/cli-tests/wafv2_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-stepfunctions` | direct | direct | direct | `simulators/aws/sdk-tests/stepfunctions_test.go`; `simulators/aws/cli-tests/stepfunctions_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-codebuild` | direct | direct | direct | `simulators/aws/sdk-tests/codebuild_test.go`; `simulators/aws/cli-tests/codebuild_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-glue` | direct | direct | direct | `simulators/aws/sdk-tests/glue_test.go`; `simulators/aws/cli-tests/glue_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `aws-batch` | direct | direct | direct | `simulators/aws/sdk-tests/batch_test.go`; `simulators/aws/cli-tests/batch_test.go`; `simulators/aws/terraform-tests/main.tf` |
| `azure-acr` | direct | direct | direct | `simulators/azure/sdk-tests/acr_test.go`; `simulators/azure/cli-tests/acr_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-cache_redis` | direct | direct | direct | `simulators/azure/sdk-tests/redis_pg_test.go`; `simulators/azure/cli-tests/redis_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-compute` | direct | direct | direct | `simulators/azure/sdk-tests/compute_test.go`; `simulators/azure/sdk-tests/network_test.go`; `simulators/azure/cli-tests/compute_test.go`; `simulators/azure/cli-tests/loadbalancer_test.go`; `simulators/azure/cli-tests/nat_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-containerinstance` | direct | direct | direct | `simulators/azure/sdk-tests/logicapps_containerinstance_test.go`; `simulators/azure/cli-tests/logicapps_containerinstance_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-containerapps` | direct | direct | direct | `simulators/azure/sdk-tests/containerapps_test.go`; `simulators/azure/cli-tests/containerapps_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-cosmos` | direct | direct | direct | `simulators/azure/sdk-tests/cosmos_test.go`; `simulators/azure/cli-tests/cosmos_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-eventgrid` | direct | direct | direct | `simulators/azure/sdk-tests/eventgrid_test.go`; `simulators/azure/cli-tests/eventgrid_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-eventhubs` | direct | direct | direct | `simulators/azure/sdk-tests/eventhub_test.go`; `simulators/azure/cli-tests/eventhub_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-functions` | direct | direct | direct | `simulators/azure/sdk-tests/functions_test.go`; `simulators/azure/cli-tests/functions_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-keyvault` | direct | direct | direct | `simulators/azure/sdk-tests/keyvault_test.go`; `simulators/azure/cli-tests/arm_foundation_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-kv-data-plane` | direct | direct | direct | `simulators/azure/sdk-tests/keyvault_sdk_test.go`; `simulators/azure/cli-tests/keyvault_dataplane_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-logicapps` | direct | direct | direct | `simulators/azure/sdk-tests/logicapps_containerinstance_test.go`; `simulators/azure/cli-tests/logicapps_containerinstance_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-monitor` | direct | direct | direct | `simulators/azure/sdk-tests/monitor_test.go`; `simulators/azure/cli-tests/monitor_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-resourcegroups` | direct | direct | direct | `simulators/azure/sdk-tests/resourcegroup_test.go`; `simulators/azure/cli-tests/arm_foundation_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-servicebus-admin` | direct | not applicable | not applicable | `simulators/azure/sdk-tests/servicebus_admin_test.go` |
| `azure-servicebus-arm` | direct | direct | direct | `simulators/azure/sdk-tests/servicebus_arm_sdk_test.go`; `simulators/azure/cli-tests/servicebus_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-servicebus-data-plane` | direct | not applicable | not applicable | `simulators/azure/sdk-tests/servicebus_dataplane_test.go` |
| `azure-storage` | direct | direct | direct | `simulators/azure/sdk-tests/storage_test.go`; `simulators/azure/cli-tests/blob_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-storage-data-plane` | direct | direct | direct | `simulators/azure/sdk-tests/storage_dataplanes_test.go`; `simulators/azure/cli-tests/blob_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `azure-subscription` | direct | direct | not applicable | `simulators/azure/sdk-tests/integration_test.go`; `simulators/azure/cli-tests/arm_foundation_test.go` |
| `azure-entra` | direct | direct | not applicable | `simulators/azure/sdk-tests/entra_test.go`; `simulators/azure/cli-tests/entra_test.go` |
| `azure-private-dns` | direct | direct | direct | `simulators/azure/sdk-tests/dns_private_test.go`; `simulators/azure/cli-tests/dns_test.go`; `simulators/azure/terraform-tests/main.tf` |
| `gcp-apigateway` | direct | direct | direct | `simulators/gcp/sdk-tests/memorystore_apigw_test.go`; `simulators/gcp/cli-tests/client_surface_audit_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-artifactregistry` | direct | direct | direct | `simulators/gcp/sdk-tests/artifactregistry_oci_test.go`; `simulators/gcp/cli-tests/artifactregistry_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-bigtable` | direct | direct | tracked BUG-1585 | `simulators/gcp/sdk-tests/spanner_dataflow_bigtable_test.go`; `simulators/gcp/cli-tests/spanner_dataflow_bigtable_test.go`; Terraform provider Bigtable Admin calls bypass the REST custom endpoint and hit real Google auth. |
| `gcp-bigquery` | direct | direct | direct | `simulators/gcp/sdk-tests/data_saas_test.go`; `simulators/gcp/cli-tests/data_saas_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-cloudbuild` | direct | direct | direct | `simulators/gcp/sdk-tests/build_test.go`; `simulators/gcp/cli-tests/client_surface_audit_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-cloudfunctions` | direct | direct | direct | `simulators/gcp/sdk-tests/functions_sdk_test.go`; `simulators/gcp/cli-tests/functions_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-cloudkms` | direct | direct | direct | `simulators/gcp/sdk-tests/cloudkms_test.go`; `simulators/gcp/cli-tests/cloudkms_test.go`; `simulators/gcp/terraform-tests/fixtures/kms-lifecycle/main.tf` |
| `gcp-cloudrun` | direct | direct | direct | `simulators/gcp/sdk-tests/run_sdk_test.go`; `simulators/gcp/cli-tests/run_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-compute` | direct | direct | direct | `simulators/gcp/sdk-tests/compute_test.go`; `simulators/gcp/cli-tests/compute_disks_test.go`; `simulators/gcp/cli-tests/compute_instances_test.go`; `simulators/gcp/cli-tests/compute_nat_test.go`; `simulators/gcp/cli-tests/client_surface_audit_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-compute_loadbalancing` | direct | direct | direct | `simulators/gcp/sdk-tests/compute_test.go`; `simulators/gcp/cli-tests/compute_loadbalancing_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-dataflow` | direct | direct | not applicable | `simulators/gcp/sdk-tests/spanner_dataflow_bigtable_test.go`; `simulators/gcp/cli-tests/spanner_dataflow_bigtable_test.go` |
| `gcp-dns` | direct | direct | direct | `simulators/gcp/sdk-tests/dns_test.go`; `simulators/gcp/cli-tests/dns_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-eventarc` | direct | direct | direct | `simulators/gcp/sdk-tests/eventarc_test.go`; `simulators/gcp/cli-tests/eventarc_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-firestore` | direct | direct | direct | `simulators/gcp/sdk-tests/data_saas_test.go`; `simulators/gcp/cli-tests/data_saas_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-gcs` | direct | direct | direct | `simulators/gcp/sdk-tests/storage_test.go`; `simulators/gcp/cli-tests/storage_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-iam` | direct | direct | direct | `simulators/gcp/sdk-tests/iam_test.go`; `simulators/gcp/cli-tests/client_surface_audit_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-logging` | direct | direct | direct | `simulators/gcp/sdk-tests/logging_test.go`; `simulators/gcp/cli-tests/logging_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-memorystore_redis` | direct | direct | direct | `simulators/gcp/sdk-tests/memorystore_apigw_test.go`; `simulators/gcp/cli-tests/redis_sql_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-pubsub` | direct | direct | direct | `simulators/gcp/sdk-tests/pubsub_test.go`; `simulators/gcp/cli-tests/client_surface_audit_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-secretmanager` | direct | direct | direct | `simulators/gcp/sdk-tests/secretmanager_test.go`; `simulators/gcp/cli-tests/secretmanager_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-sqladmin` | direct | direct | direct | `simulators/gcp/sdk-tests/cloudsql_test.go`; `simulators/gcp/cli-tests/redis_sql_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-spanner` | direct | direct | direct | `simulators/gcp/sdk-tests/spanner_dataflow_bigtable_test.go`; `simulators/gcp/cli-tests/spanner_dataflow_bigtable_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `gcp-vpcaccess` | direct | direct | direct | `simulators/gcp/sdk-tests/integration_test.go`; `simulators/gcp/cli-tests/vpcaccess_test.go`; `simulators/gcp/terraform-tests/main.tf` |
| `bleephub-actions` | direct | direct | not applicable | `bleephub/gh_actions_test.go`; `bleephub/gh_workflows_test.go` |
| `bleephub-apps` | direct | direct | not applicable | `bleephub/gh_apps_test.go`; `bleephub/gh_apps_more_test.go`; `bleephub/gh_apps_events_test.go`; `bleephub/gh_apps_oauth_mgmt_test.go`; `bleephub/gh_apps_perms_test.go`; `bleephub/gh_app_hooks_test.go`; `bleephub/gh_oauth_test.go`; `bleephub/gh_user_installations_test.go` |
| `bleephub-checks` | direct | direct | not applicable | `bleephub/gh_checks_test.go` |
| `bleephub-deployments` | direct | direct | not applicable | `bleephub/gh_deployments_test.go` |
| `bleephub-hooks` | direct | direct | not applicable | `bleephub/gh_hooks_test.go` |
| `bleephub-issues` | direct | direct | not applicable | `bleephub/gh_issues_test.go`; `bleephub/gh_reactions_test.go` |
| `bleephub-orgs` | direct | direct | not applicable | `bleephub/gh_orgs_test.go` |
| `bleephub-pulls` | direct | direct | not applicable | `bleephub/gh_pulls_test.go`; `bleephub/gh_pr_comments_test.go` |
| `bleephub-releases` | direct | direct | not applicable | `bleephub/gh_releases_test.go` |
| `bleephub-repos` | direct | direct | not applicable | `bleephub/gh_repos_test.go` |
| `bleephub-teams` | direct | direct | not applicable | `bleephub/gh_orgs_test.go` |
| `bleephub-users` | direct | direct | not applicable | `bleephub/gh_test.go`; `bleephub/gh_misc_endpoints_decode_test.go`; `bleephub/gh_actions_test.go` |
