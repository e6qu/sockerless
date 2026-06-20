package main

import "testing"

// FuzzAzureResourceIDParsers fuzzes the ARM resource-id path extractors. These
// run over an untrusted resource ID drawn from request bodies (@odata.id,
// cross-resource references) and path params, and must never panic on a
// malformed `/subscriptions/X/resourceGroups/Y/providers/...` string.
func FuzzAzureResourceIDParsers(f *testing.F) {
	seeds := []string{
		"",
		"/",
		"//",
		"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.DocumentDB/databaseAccounts/acc1",
		"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.DocumentDB/databaseAccounts/acc1/sqlDatabases/db1/containers/c1",
		"subscriptions",
		"/subscriptions/",
		"resourceGroups/",
		"/subscriptions/sub1/resourceGroups",
		"databaseAccounts/sqlDatabases/containers",
		"/////////databaseAccounts",
		"\x00\xff/subscriptions/\x00",
		"databaseAccounts/",
		"sqlDatabases/",
		"containers/",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, id string) {
		// None of these may panic on any input.
		_ = azureSubscriptionFromID(id, "default-sub")
		_ = azureResourceGroupFromID(id, "default-rg")
		_, _, _, _ = cosmosARMIDNames(id)
	})
}

// FuzzAzureStorageAddressParsers fuzzes the storage/messaging address parsers
// that decompose an untrusted Host header or AMQP address into account /
// container / blob / hub / partition components. A malformed host or address
// must produce ok=false, never a panic.
func FuzzAzureStorageAddressParsers(f *testing.F) {
	seeds := []string{
		"",
		"/",
		"acct.blob.localhost/container/blob",
		"https://acct.blob.localhost:4568/c/b",
		"acct.blob.",
		".blob.",
		"acct.blob.localhost/",
		"http://[::1]/x",
		":",
		"hub/Partitions/0",
		"cg/ConsumerGroups/x/Partitions/3",
		"a/b/c/d/e/f",
		"\x00.blob.\x00",
		"acct.blob.localhost/%ZZ/blob",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _, _, _ = parseBlobCopySource(s)
		_, _, _ = splitContainerBlobPath(s)
		_, _, _, _ = parseACRBlobURL(s)
		_, _, _ = ehAMQPParseEventHubAddress(s)
		_, _, _ = ehAMQPParseConsumerAddress(s)
	})
}

// FuzzCosmosEqualityQuery fuzzes the Cosmos SQL equality-query parser, which
// runs over an untrusted SQL query string from the Cosmos data plane. It must
// never panic regardless of how malformed the WHERE clause is.
func FuzzCosmosEqualityQuery(f *testing.F) {
	seeds := []string{
		"",
		"SELECT * FROM c",
		"SELECT * FROM c WHERE c.id = 'x'",
		"SELECT * FROM c WHERE c.id = @id",
		"where",
		"where =",
		"where ===",
		"WHERE c.a = b = c",
		"where c.id=",
		"\x00where\x00=\x00",
		"WhErE   c.[`x`]   =   'v'",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, query string) {
		params := []map[string]any{{"name": "@id", "value": "x"}}
		_, _, _ = cosmosParseEqualityQuery(query, params)
	})
}
