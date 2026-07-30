module github.com/sockerless/simulator-aws-sdk-tests

go 1.25.0

require (
	github.com/aws/aws-sdk-go-v2 v1.43.2
	github.com/aws/aws-sdk-go-v2/config v1.32.33
	github.com/aws/aws-sdk-go-v2/credentials v1.19.32
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.33
	github.com/aws/aws-sdk-go-v2/feature/rds/auth v1.6.33
	github.com/aws/aws-sdk-go-v2/service/acm v1.43.2
	github.com/aws/aws-sdk-go-v2/service/acmpca v1.49.2
	github.com/aws/aws-sdk-go-v2/service/amplify v1.41.2
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.42.2
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.37.2
	github.com/aws/aws-sdk-go-v2/service/applicationautoscaling v1.45.2
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.70.2
	github.com/aws/aws-sdk-go-v2/service/batch v1.68.2
	github.com/aws/aws-sdk-go-v2/service/budgets v1.46.2
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.67.2
	github.com/aws/aws-sdk-go-v2/service/cloudtrail v1.58.2
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.66.1
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.80.2
	github.com/aws/aws-sdk-go-v2/service/codebuild v1.72.2
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.62.2
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.318.0
	github.com/aws/aws-sdk-go-v2/service/ecr v1.60.2
	github.com/aws/aws-sdk-go-v2/service/ecs v1.89.2
	github.com/aws/aws-sdk-go-v2/service/efs v1.44.2
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.56.2
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.58.3
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.48.2
	github.com/aws/aws-sdk-go-v2/service/firehose v1.46.2
	github.com/aws/aws-sdk-go-v2/service/glue v1.151.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.56.2
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.46.2
	github.com/aws/aws-sdk-go-v2/service/kms v1.55.2
	github.com/aws/aws-sdk-go-v2/service/lambda v1.100.2
	github.com/aws/aws-sdk-go-v2/service/organizations v1.53.2
	github.com/aws/aws-sdk-go-v2/service/rds v1.123.2
	github.com/aws/aws-sdk-go-v2/service/route53 v1.65.4
	github.com/aws/aws-sdk-go-v2/service/s3 v1.106.2
	github.com/aws/aws-sdk-go-v2/service/scheduler v1.20.2
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.44.2
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.43.2
	github.com/aws/aws-sdk-go-v2/service/sfn v1.45.2
	github.com/aws/aws-sdk-go-v2/service/sns v1.42.2
	github.com/aws/aws-sdk-go-v2/service/sqs v1.46.2
	github.com/aws/aws-sdk-go-v2/service/ssm v1.73.2
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.2
	github.com/aws/aws-sdk-go-v2/service/wafv2 v1.77.0
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
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.34 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.26 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.12.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.33 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.34 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	golang.org/x/text v0.40.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
