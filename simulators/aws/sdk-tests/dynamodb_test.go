package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ddbClient() *dynamodb.Client {
	return dynamodb.NewFromConfig(sdkConfig(), func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestDynamoDB_TableLifecycle covers Create/Describe/List/Delete on
// the table surface terraform's `aws_dynamodb_table` exercises.
func TestDynamoDB_TableLifecycle(t *testing.T) {
	c := ddbClient()
	tableName := "runner-state-test"

	defer c.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})

	createOut, err := c.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("LockID"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("LockID"), KeyType: ddbtypes.KeyTypeHash},
		},
		BillingMode: ddbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.TableDescription)
	assert.Equal(t, "ACTIVE", string(createOut.TableDescription.TableStatus))
	assert.Contains(t, *createOut.TableDescription.TableArn, "arn:aws:dynamodb:")

	desc, err := c.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	require.NoError(t, err)
	assert.Equal(t, tableName, *desc.Table.TableName)

	list, err := c.ListTables(ctx, &dynamodb.ListTablesInput{})
	require.NoError(t, err)
	assert.Contains(t, list.TableNames, tableName)
}

// TestDynamoDB_TerraformStateLockSemantics pins the canonical
// state-lock acquire/release flow that terraform uses with
// `backend "s3" { dynamodb_table = "..." }` — PutItem with
// ConditionExpression="attribute_not_exists(LockID)" must succeed
// the first time and fail with ConditionalCheckFailedException on
// the second concurrent acquire.
func TestDynamoDB_TerraformStateLockSemantics(t *testing.T) {
	c := ddbClient()
	tableName := "tf-lock-test"

	_, err := c.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("LockID"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("LockID"), KeyType: ddbtypes.KeyTypeHash},
		},
		BillingMode: ddbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err)
	defer c.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})

	// First acquire — should succeed.
	_, err = c.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: "tf-state-lock"},
			"Owner":  &ddbtypes.AttributeValueMemberS{Value: "runner-1"},
		},
		ConditionExpression: aws.String("attribute_not_exists(LockID)"),
	})
	require.NoError(t, err, "first PutItem must succeed when no item exists")

	// Second acquire — must fail with the ConditionalCheckFailed shape
	// terraform translates into "lock contention".
	_, err = c.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: "tf-state-lock"},
			"Owner":  &ddbtypes.AttributeValueMemberS{Value: "runner-2"},
		},
		ConditionExpression: aws.String("attribute_not_exists(LockID)"),
	})
	require.Error(t, err, "second PutItem must fail with ConditionalCheckFailedException")

	// GetItem returns the original item (held by runner-1).
	got, err := c.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: "tf-state-lock"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, got.Item)
	owner := got.Item["Owner"].(*ddbtypes.AttributeValueMemberS)
	assert.Equal(t, "runner-1", owner.Value)

	// Release the lock.
	_, err = c.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: "tf-state-lock"},
		},
	})
	require.NoError(t, err)

	// New runner can now acquire.
	_, err = c.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]ddbtypes.AttributeValue{
			"LockID": &ddbtypes.AttributeValueMemberS{Value: "tf-state-lock"},
			"Owner":  &ddbtypes.AttributeValueMemberS{Value: "runner-3"},
		},
		ConditionExpression: aws.String("attribute_not_exists(LockID)"),
	})
	require.NoError(t, err, "PutItem after Delete must succeed")
}

func TestDynamoDB_QueryAndScan(t *testing.T) {
	c := ddbClient()
	tableName := "query-scan-test"
	_, err := c.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("ID"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("ID"), KeyType: ddbtypes.KeyTypeHash},
		},
		BillingMode: ddbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err)
	defer c.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})

	for _, id := range []string{"a", "b", "c"} {
		_, err = c.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item: map[string]ddbtypes.AttributeValue{
				"ID": &ddbtypes.AttributeValueMemberS{Value: id},
			},
		})
		require.NoError(t, err)
	}

	scan, err := c.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(tableName)})
	require.NoError(t, err)
	assert.Equal(t, int32(3), scan.Count)
}
