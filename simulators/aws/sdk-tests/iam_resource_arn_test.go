package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIAM_ResourceARN_DynamoDB proves the enforcement gate derives a request's
// resource ARN for an awsJson service (DynamoDB, from the body's TableName), so a
// policy scoped to one table's ARN allows that table and denies another.
func TestIAM_ResourceARN_DynamoDB(t *testing.T) {
	admin := iamClient()
	ddbAdmin := dynamodb.NewFromConfig(sdkConfig(), func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(baseURL) })

	mkTable := func(name string) {
		_, err := ddbAdmin.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName:            aws.String(name),
			BillingMode:          ddbtypes.BillingModePayPerRequest,
			AttributeDefinitions: []ddbtypes.AttributeDefinition{{AttributeName: aws.String("id"), AttributeType: ddbtypes.ScalarAttributeTypeS}},
			KeySchema:            []ddbtypes.KeySchemaElement{{AttributeName: aws.String("id"), KeyType: ddbtypes.KeyTypeHash}},
		})
		require.NoError(t, err)
	}
	mkTable("scoped-table")
	mkTable("other-table")

	user := "ddb-scoped-user"
	_, err := admin.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(user)})
	require.NoError(t, err)
	defer admin.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(user)})
	_, err = admin.PutUserPolicy(ctx, &iam.PutUserPolicyInput{
		UserName:   aws.String(user),
		PolicyName: aws.String("one-table"),
		PolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"dynamodb:PutItem",` +
			`"Resource":"arn:aws:dynamodb:us-east-1:123456789012:table/scoped-table"}]}`),
	})
	require.NoError(t, err)
	key, err := admin.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{UserName: aws.String(user)})
	require.NoError(t, err)

	cfg := aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider(
		aws.ToString(key.AccessKey.AccessKeyId), aws.ToString(key.AccessKey.SecretAccessKey), "")}
	ddb := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(baseURL) })

	item := map[string]ddbtypes.AttributeValue{"id": &ddbtypes.AttributeValueMemberS{Value: "x"}}
	_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String("scoped-table"), Item: item})
	assert.NoError(t, err, "PutItem on the granted table ARN must succeed")

	_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String("other-table"), Item: item})
	require.Error(t, err, "PutItem on a different table must be denied by the resource-scoped policy")
	assert.Equal(t, "AccessDeniedException", errCodeOf(err))
}
