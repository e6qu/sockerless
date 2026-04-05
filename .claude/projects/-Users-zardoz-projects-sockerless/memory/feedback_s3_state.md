---
name: S3 state bucket preferences
description: No versioning on terraform state S3 buckets, use native S3 locks not DynamoDB
type: feedback
---

Do not enable versioning on S3 state buckets. Use native S3 locking (use_lockfile), not DynamoDB tables, for terraform state locking.

**Why:** User preference for simpler, cheaper state management.
**How to apply:** When creating terraform remote state configs, omit dynamodb_table and bucket versioning. Use `use_lockfile = true` when terraform version supports it.
