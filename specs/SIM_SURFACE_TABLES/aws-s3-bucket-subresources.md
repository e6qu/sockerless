# AWS S3 — bucket-level subresources

Surface: `simulators/aws/s3.go` + `simulators/aws/s3_bucket_subresources.go`. Every operation listed is `<verb> /{bucket}?<subresource>` against the canonical S3 endpoint.

Canonical reference: <https://docs.aws.amazon.com/AmazonS3/latest/API/API_Operations_Amazon_Simple_Storage_Service.html>

## Status legend

- ✓ — implemented + tested
- ✗ — missing
- 501 — stubbed with `NotImplemented` envelope (wire-visible gap)
- n/a — no terraform-provider resource exists for this subresource

## Versioning, lifecycle, configuration

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|
| PutBucketVersioning | `PUT /{bucket}?versioning` | ✓ `s3_bucket_subresources.go::handleS3PutBucketSubresource` | ✓ `s3_bucket_subresources_test.go::TestS3_Bucket_Versioning_RoundTrip` | ✗ | tf: `aws_s3_bucket_versioning` |
| GetBucketVersioning | `GET /{bucket}?versioning` | ✓ `s3.go::handleS3GetBucket` | ✓ same | ✗ | |
| PutBucketLifecycleConfiguration | `PUT /{bucket}?lifecycle` | ✓ same | ✓ `TestS3_Bucket_Lifecycle_RoundTrip` | ✗ | tf: `aws_s3_bucket_lifecycle_configuration` |
| GetBucketLifecycleConfiguration | `GET /{bucket}?lifecycle` | ✓ same | ✓ same | ✗ | |
| DeleteBucketLifecycle | `DELETE /{bucket}?lifecycle` | ✓ `handleS3DeleteBucketSubresource` | ✓ same | ✗ | |
| PutBucketCors | `PUT /{bucket}?cors` | ✓ same | ✓ `TestS3_Bucket_Cors_RoundTrip` | ✗ | tf: `aws_s3_bucket_cors_configuration` |
| GetBucketCors | `GET /{bucket}?cors` | ✓ same | ✓ same | ✗ | |
| DeleteBucketCors | `DELETE /{bucket}?cors` | ✓ same | ✓ same | ✗ | |
| PutBucketPolicy | `PUT /{bucket}?policy` | ✓ same | ✓ `TestS3_Bucket_Policy_RoundTrip` | ✗ | tf: `aws_s3_bucket_policy` |
| GetBucketPolicy | `GET /{bucket}?policy` | ✓ same | ✓ same | ✗ | |
| DeleteBucketPolicy | `DELETE /{bucket}?policy` | ✓ same | ✓ same | ✗ | |
| PutBucketEncryption | `PUT /{bucket}?encryption` | ✓ same | ✓ `TestS3_Bucket_Encryption_RoundTrip` | ✗ | tf: `aws_s3_bucket_server_side_encryption_configuration` |
| GetBucketEncryption | `GET /{bucket}?encryption` | ✓ same | ✓ same | ✗ | |
| DeleteBucketEncryption | `DELETE /{bucket}?encryption` | ✓ same | ✓ same | ✗ | |
| PutBucketReplication | `PUT /{bucket}?replication` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_replication_configuration` — needs Source role + destination bucket; deferred to a later phase. |
| GetBucketReplication | `GET /{bucket}?replication` | ✓ same | ✗ | ✗ | |
| DeleteBucketReplication | `DELETE /{bucket}?replication` | ✓ same | ✗ | ✗ | |
| PutBucketTagging | `PUT /{bucket}?tagging` | ✓ same | ✓ `TestS3_Bucket_Tagging_RoundTrip` | ✗ | tf: managed via `aws_s3_bucket.tags` |
| GetBucketTagging | `GET /{bucket}?tagging` | ✓ same | ✓ same | ✗ | |
| DeleteBucketTagging | `DELETE /{bucket}?tagging` | ✓ same | ✓ same | ✗ | |
| PutBucketWebsite | `PUT /{bucket}?website` | ✓ same | ✓ `TestS3_Bucket_Website_RoundTrip` | ✗ | tf: `aws_s3_bucket_website_configuration` |
| GetBucketWebsite | `GET /{bucket}?website` | ✓ same | ✓ same | ✗ | |
| DeleteBucketWebsite | `DELETE /{bucket}?website` | ✓ same | ✓ same | ✗ | |
| PutBucketLogging | `PUT /{bucket}?logging` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_logging` |
| GetBucketLogging | `GET /{bucket}?logging` | ✓ same | ✗ | ✗ | |
| PutBucketAcl | `PUT /{bucket}?acl` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_acl` |
| GetBucketAcl | `GET /{bucket}?acl` | ✓ same | ✗ | ✗ | |
| PutBucketRequestPayment | `PUT /{bucket}?requestPayment` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_request_payment_configuration` |
| GetBucketRequestPayment | `GET /{bucket}?requestPayment` | ✓ same | ✗ | ✗ | |
| PutBucketAccelerateConfiguration | `PUT /{bucket}?accelerate` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_accelerate_configuration` |
| GetBucketAccelerateConfiguration | `GET /{bucket}?accelerate` | ✓ same | ✗ | ✗ | |
| PutBucketOwnershipControls | `PUT /{bucket}?ownershipControls` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_ownership_controls` |
| GetBucketOwnershipControls | `GET /{bucket}?ownershipControls` | ✓ same | ✗ | ✗ | |
| DeleteBucketOwnershipControls | `DELETE /{bucket}?ownershipControls` | ✓ same | ✗ | ✗ | |
| PutBucketNotificationConfiguration | `PUT /{bucket}?notification` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_notification` |
| GetBucketNotificationConfiguration | `GET /{bucket}?notification` | ✓ same | ✗ | ✗ | |
| PutPublicAccessBlock | `PUT /{bucket}?publicAccessBlock` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_public_access_block` |
| GetPublicAccessBlock | `GET /{bucket}?publicAccessBlock` | ✓ same | ✗ | ✗ | |
| DeletePublicAccessBlock | `DELETE /{bucket}?publicAccessBlock` | ✓ same | ✗ | ✗ | |
| PutObjectLockConfiguration | `PUT /{bucket}?object-lock` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_object_lock_configuration` |
| GetObjectLockConfiguration | `GET /{bucket}?object-lock` | ✓ same | ✗ | ✗ | |
| PutBucketIntelligentTieringConfiguration | `PUT /{bucket}?intelligent-tiering&id={id}` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_intelligent_tiering_configuration` |
| ListBucketIntelligentTieringConfigurations | `GET /{bucket}?intelligent-tiering` | ✓ same | ✗ | ✗ | |
| DeleteBucketIntelligentTieringConfiguration | `DELETE /{bucket}?intelligent-tiering&id={id}` | ✓ same | ✗ | ✗ | |
| PutBucketInventoryConfiguration | `PUT /{bucket}?inventory&id={id}` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_inventory` |
| ListBucketInventoryConfigurations | `GET /{bucket}?inventory` | ✓ same | ✗ | ✗ | |
| DeleteBucketInventoryConfiguration | `DELETE /{bucket}?inventory&id={id}` | ✓ same | ✗ | ✗ | |
| PutBucketAnalyticsConfiguration | `PUT /{bucket}?analytics&id={id}` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_analytics_configuration` |
| ListBucketAnalyticsConfigurations | `GET /{bucket}?analytics` | ✓ same | ✗ | ✗ | |
| DeleteBucketAnalyticsConfiguration | `DELETE /{bucket}?analytics&id={id}` | ✓ same | ✗ | ✗ | |
| PutBucketMetricsConfiguration | `PUT /{bucket}?metrics&id={id}` | ✓ same | ✗ | ✗ | tf: `aws_s3_bucket_metric` |
| ListBucketMetricsConfigurations | `GET /{bucket}?metrics` | ✓ same | ✗ | ✗ | |
| DeleteBucketMetricsConfiguration | `DELETE /{bucket}?metrics&id={id}` | ✓ same | ✗ | ✗ | |
| GetBucketLocation | `GET /{bucket}?location` | ✓ same | ✗ | ✗ | |
| GetBucketPolicyStatus | `GET /{bucket}?policyStatus` | ✓ same | ✗ | ✗ | |

## Bucket lifecycle (Create / Delete / List)

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|
| CreateBucket | `PUT /{bucket}` (no subresource) | ✓ `s3.go::handleS3CreateBucket` | ✓ existing | ✓ `aws_s3_bucket` | |
| HeadBucket | `HEAD /{bucket}` | ✓ same | ✗ | ✗ | |
| DeleteBucket | `DELETE /{bucket}` (no subresource) | ✓ same | ✗ | ✗ | |
| ListBuckets | `GET /` | ✓ `s3.go::handleS3ListBuckets` | ✗ | ✗ | |

## Open subtasks staged forward

- Add tf-tests for at least the high-fanout subresources (`?versioning`, `?lifecycle`, `?cors`, `?policy`, `?encryption`, `?tagging`, `?website`) — tracked under BUG-1147 in the same PR.
- Replication round-trip test — needs source-bucket + destination-bucket fixture + IAM role; deferred until a runner scenario exercises it.

## Reopens that produced this table

- Issue [#201](https://github.com/e6qu/sockerless/issues/201) — bucket-level PUT subresources routed to CreateBucket → 409 BucketAlreadyOwnedByYou. PR #200's `s3_subresources.go` only covered the object-level PUT/POST surface the user named. This table exists so the next reopen of this shape never repeats.
