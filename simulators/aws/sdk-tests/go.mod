module github.com/sockerless/simulator-aws-sdk-tests

go 1.25.0

require (
	github.com/aws/aws-sdk-go-v2 v1.43.1
	github.com/aws/aws-sdk-go-v2/config v1.32.32
	github.com/aws/aws-sdk-go-v2/credentials v1.19.31
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.32
	github.com/aws/aws-sdk-go-v2/feature/rds/auth v1.6.32
	github.com/aws/aws-sdk-go-v2/service/acm v1.43.1
	github.com/aws/aws-sdk-go-v2/service/acmpca v1.49.1
	github.com/aws/aws-sdk-go-v2/service/amplify v1.41.1
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.42.1
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.37.1
	github.com/aws/aws-sdk-go-v2/service/applicationautoscaling v1.45.1
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.70.1
	github.com/aws/aws-sdk-go-v2/service/batch v1.68.1
	github.com/aws/aws-sdk-go-v2/service/budgets v1.46.1
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.67.1
	github.com/aws/aws-sdk-go-v2/service/cloudtrail v1.58.1
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.66.0
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.80.1
	github.com/aws/aws-sdk-go-v2/service/codebuild v1.72.1
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.62.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.317.1
	github.com/aws/aws-sdk-go-v2/service/ecr v1.60.1
	github.com/aws/aws-sdk-go-v2/service/ecs v1.89.1
	github.com/aws/aws-sdk-go-v2/service/efs v1.44.1
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.56.1
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.58.2
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.48.1
	github.com/aws/aws-sdk-go-v2/service/firehose v1.46.1
	github.com/aws/aws-sdk-go-v2/service/glue v1.150.1
	github.com/aws/aws-sdk-go-v2/service/iam v1.56.1
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.46.1
	github.com/aws/aws-sdk-go-v2/service/kms v1.55.1
	github.com/aws/aws-sdk-go-v2/service/lambda v1.100.1
	github.com/aws/aws-sdk-go-v2/service/organizations v1.53.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.123.1
	github.com/aws/aws-sdk-go-v2/service/route53 v1.65.3
	github.com/aws/aws-sdk-go-v2/service/s3 v1.106.1
	github.com/aws/aws-sdk-go-v2/service/scheduler v1.20.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.44.1
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.43.1
	github.com/aws/aws-sdk-go-v2/service/sfn v1.45.1
	github.com/aws/aws-sdk-go-v2/service/sns v1.42.1
	github.com/aws/aws-sdk-go-v2/service/sqs v1.46.1
	github.com/aws/aws-sdk-go-v2/service/ssm v1.73.1
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.1
	github.com/aws/aws-sdk-go-v2/service/wafv2 v1.76.1
	github.com/aws/smithy-go v1.27.5
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/go-sql-driver/mysql v1.10.0
	github.com/golang/snappy v1.0.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/jackc/pgx/v5 v5.10.0
	github.com/sockerless/simulator-testutil v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.54.0
	golang.org/x/net v0.57.0
)

replace github.com/sockerless/simulator-testutil => ../../testutil

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.15 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.33 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.12.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.33 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	golang.org/x/text v0.40.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
