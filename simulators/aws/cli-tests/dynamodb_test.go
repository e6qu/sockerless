package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamoDBCLI_TableAndItems(t *testing.T) {
	table := "cli-ddb-table"

	runCLI(t, awsCLI("dynamodb", "create-table",
		"--table-name", table,
		"--attribute-definitions", "AttributeName=pk,AttributeType=S",
		"--key-schema", "AttributeName=pk,KeyType=HASH",
		"--billing-mode", "PAY_PER_REQUEST"))
	t.Cleanup(func() {
		_ = awsCLI("dynamodb", "delete-table", "--table-name", table).Run()
	})

	out := runCLI(t, awsCLI("dynamodb", "describe-table", "--table-name", table))
	var desc struct {
		Table struct {
			TableName   string `json:"TableName"`
			TableStatus string `json:"TableStatus"`
		} `json:"Table"`
	}
	parseJSON(t, out, &desc)
	assert.Equal(t, table, desc.Table.TableName)
	assert.Equal(t, "ACTIVE", desc.Table.TableStatus)

	runCLI(t, awsCLI("dynamodb", "put-item",
		"--table-name", table,
		"--item", `{"pk":{"S":"a"},"kind":{"S":"wanted"}}`))
	runCLI(t, awsCLI("dynamodb", "put-item",
		"--table-name", table,
		"--item", `{"pk":{"S":"b"},"kind":{"S":"ignored"}}`))

	out = runCLI(t, awsCLI("dynamodb", "get-item",
		"--table-name", table,
		"--key", `{"pk":{"S":"a"}}`))
	var got struct {
		Item map[string]map[string]string `json:"Item"`
	}
	parseJSON(t, out, &got)
	require.Equal(t, "wanted", got.Item["kind"]["S"])

	out = runCLI(t, awsCLI("dynamodb", "query",
		"--table-name", table,
		"--key-condition-expression", "#pk = :pk",
		"--expression-attribute-names", `{"#pk":"pk"}`,
		"--expression-attribute-values", `{":pk":{"S":"a"}}`))
	var query struct {
		Items []map[string]map[string]string `json:"Items"`
		Count int                            `json:"Count"`
	}
	parseJSON(t, out, &query)
	require.Equal(t, 1, query.Count)
	require.Equal(t, "a", query.Items[0]["pk"]["S"])
}
