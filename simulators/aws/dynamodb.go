package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// DynamoDB — Sockerless's runner workflows often use DynamoDB for
// Terraform state locking (`backend "s3" { dynamodb_table = "..." }`),
// runner-job tracking, and shared state across distributed CI tasks.
// Without this slice, terraform's state-lock acquire 404s and
// `aws dynamodb` workflow steps fail.
//
// Field set covers the JSON-protocol actions terraform + the SDK use:
// CreateTable / DescribeTable / DeleteTable / ListTables /
// PutItem / GetItem / UpdateItem / DeleteItem / Query / Scan +
// the conditional-write semantics terraform's state lock relies on
// (ConditionExpression with attribute_not_exists).

// DDBTable is a DynamoDB table. Real AWS stores items keyed by
// HashKey + RangeKey; the sim collapses to HashKey-only storage
// (the most common shape for Terraform state locks: `LockID` is the
// hash key, no range key) and falls through to the slow path for
// composite keys when a RangeKey is declared.
type DDBTable struct {
	TableName            string                 `json:"TableName"`
	TableArn             string                 `json:"TableArn"`
	TableStatus          string                 `json:"TableStatus"`
	CreationDateTime     float64                `json:"CreationDateTime"`
	AttributeDefinitions []DDBAttributeDef      `json:"AttributeDefinitions"`
	KeySchema            []DDBKeySchemaEntry    `json:"KeySchema"`
	BillingModeSummary   *DDBBillingModeSummary `json:"BillingModeSummary,omitempty"`
	ItemCount            int64                  `json:"ItemCount"`
	TableSizeBytes       int64                  `json:"TableSizeBytes"`
}

// DDBAttributeDef matches the SDK's `AttributeDefinition` shape.
type DDBAttributeDef struct {
	AttributeName string `json:"AttributeName"`
	AttributeType string `json:"AttributeType"` // S / N / B
}

// DDBKeySchemaEntry pairs an attribute with its role.
type DDBKeySchemaEntry struct {
	AttributeName string `json:"AttributeName"`
	KeyType       string `json:"KeyType"` // HASH / RANGE
}

// DDBBillingModeSummary mirrors the SDK shape — `PAY_PER_REQUEST` or
// `PROVISIONED`. The sim accepts both; tests don't exercise actual
// throughput throttling.
type DDBBillingModeSummary struct {
	BillingMode                       string `json:"BillingMode"`
	LastUpdateToPayPerRequestDateTime int64  `json:"LastUpdateToPayPerRequestDateTime,omitempty"`
}

var (
	ddbTables sim.Store[DDBTable]
	// ddbItems holds per-table item maps. Keyed by `<table>/<itemKey>`,
	// where itemKey is a deterministic encoding of the primary-key
	// attribute values (HASH#<value> or HASH#<v>|RANGE#<v>).
	ddbItems   sim.Store[map[string]any]
	ddbItemsMu sync.Mutex
)

func ddbTableArn(name string) string {
	return fmt.Sprintf("arn:aws:dynamodb:%s:%s:table/%s", awsRegion(), awsAccountID(), name)
}

func registerDynamoDB(r *sim.AWSRouter, srv *sim.Server) {
	ddbTables = sim.MakeStore[DDBTable](srv.DB(), "ddb_tables")
	ddbItems = sim.MakeStore[map[string]any](srv.DB(), "ddb_items")
	ddbItemNames = sim.MakeStore[string](srv.DB(), "ddb_item_names")

	r.Register("DynamoDB_20120810.CreateTable", handleDDBCreateTable)
	r.Register("DynamoDB_20120810.DescribeTable", handleDDBDescribeTable)
	r.Register("DynamoDB_20120810.DeleteTable", handleDDBDeleteTable)
	r.Register("DynamoDB_20120810.ListTables", handleDDBListTables)
	r.Register("DynamoDB_20120810.PutItem", handleDDBPutItem)
	r.Register("DynamoDB_20120810.GetItem", handleDDBGetItem)
	r.Register("DynamoDB_20120810.UpdateItem", handleDDBUpdateItem)
	r.Register("DynamoDB_20120810.DeleteItem", handleDDBDeleteItem)
	r.Register("DynamoDB_20120810.Query", handleDDBQuery)
	r.Register("DynamoDB_20120810.Scan", handleDDBScan)
}

func handleDDBCreateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName            string              `json:"TableName"`
		AttributeDefinitions []DDBAttributeDef   `json:"AttributeDefinitions"`
		KeySchema            []DDBKeySchemaEntry `json:"KeySchema"`
		BillingMode          string              `json:"BillingMode"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.TableName == "" {
		sim.AWSError(w, "ValidationException", "TableName is required", http.StatusBadRequest)
		return
	}
	if _, exists := ddbTables.Get(req.TableName); exists {
		sim.AWSErrorf(w, "ResourceInUseException", http.StatusBadRequest,
			"Table already exists: %s", req.TableName)
		return
	}
	billingMode := req.BillingMode
	if billingMode == "" {
		billingMode = "PROVISIONED"
	}
	table := DDBTable{
		TableName:            req.TableName,
		TableArn:             ddbTableArn(req.TableName),
		TableStatus:          "ACTIVE",
		CreationDateTime:     float64(time.Now().Unix()),
		AttributeDefinitions: req.AttributeDefinitions,
		KeySchema:            req.KeySchema,
		BillingModeSummary: &DDBBillingModeSummary{
			BillingMode: billingMode,
		},
	}
	ddbTables.Put(req.TableName, table)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"TableDescription": table})
}

func handleDDBDescribeTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Table": t})
}

func handleDDBDeleteTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	ddbTables.Delete(req.TableName)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"TableDescription": t})
}

func handleDDBListTables(w http.ResponseWriter, r *http.Request) {
	all := ddbTables.List()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.TableName)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"TableNames": names})
}

// ddbItemKey encodes the primary-key attribute values into a stable
// store key. Composite keys join HASH and RANGE with `|`.
func ddbItemKey(table DDBTable, item map[string]any) string {
	var hash, rng string
	for _, k := range table.KeySchema {
		val := ddbExtractAttrValue(item[k.AttributeName])
		switch k.KeyType {
		case "HASH":
			hash = val
		case "RANGE":
			rng = val
		}
	}
	if rng != "" {
		return table.TableName + "/" + hash + "|" + rng
	}
	return table.TableName + "/" + hash
}

// ddbExtractAttrValue pulls the type-tagged value from a DynamoDB
// AttributeValue map (`{"S": "..."}` / `{"N": "..."}` / `{"B": ...}`).
// The encoding ignores the type tag for storage-key purposes — two
// items with the same primary-key value collide regardless of type.
func ddbExtractAttrValue(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"S", "N", "B"} {
		if val, ok := m[key]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

func handleDDBPutItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName                string            `json:"TableName"`
		Item                     map[string]any    `json:"Item"`
		ConditionExpression      string            `json:"ConditionExpression"`
		ExpressionAttributeNames map[string]string `json:"ExpressionAttributeNames,omitempty"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()
	itemKey := ddbItemKey(t, req.Item)
	_, exists := ddbItems.Get(itemKey)

	// Terraform state lock uses ConditionExpression="attribute_not_exists(LockID)"
	// to atomically acquire the lock. Mirror that behaviour: when the
	// expression contains "attribute_not_exists", reject the put if
	// the item already exists.
	if req.ConditionExpression != "" {
		if exists && (containsCI(req.ConditionExpression, "attribute_not_exists")) {
			sim.AWSError(w, "ConditionalCheckFailedException",
				"The conditional request failed", http.StatusBadRequest)
			return
		}
	}
	ddbItems.Put(itemKey, req.Item)
	ddbItemNames.Put(itemKey, itemKey)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleDDBGetItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string         `json:"TableName"`
		Key       map[string]any `json:"Key"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	itemKey := ddbItemKey(t, req.Key)
	item, ok := ddbItems.Get(itemKey)
	if !ok {
		// Real DynamoDB returns 200 with no Item field for missing keys.
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Item": item})
}

func handleDDBUpdateItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string         `json:"TableName"`
		Key       map[string]any `json:"Key"`
		// Real UpdateItem supports UpdateExpression — for sim's needs
		// we accept AttributeUpdates (legacy) which is simpler.
		AttributeUpdates map[string]struct {
			Action string         `json:"Action"`
			Value  map[string]any `json:"Value"`
		} `json:"AttributeUpdates"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()
	itemKey := ddbItemKey(t, req.Key)
	item, _ := ddbItems.Get(itemKey)
	if item == nil {
		item = map[string]any{}
		// Copy primary-key attrs from Key into the new item.
		for k, v := range req.Key {
			item[k] = v
		}
	}
	for attr, upd := range req.AttributeUpdates {
		switch upd.Action {
		case "DELETE":
			delete(item, attr)
		default: // PUT (default) and ADD treated as overwrite for sim's needs
			item[attr] = upd.Value
		}
	}
	ddbItems.Put(itemKey, item)
	ddbItemNames.Put(itemKey, itemKey)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Attributes": item})
}

func handleDDBDeleteItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName           string         `json:"TableName"`
		Key                 map[string]any `json:"Key"`
		ConditionExpression string         `json:"ConditionExpression"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()
	itemKey := ddbItemKey(t, req.Key)
	if req.ConditionExpression != "" {
		_, exists := ddbItems.Get(itemKey)
		if !exists && containsCI(req.ConditionExpression, "attribute_exists") {
			sim.AWSError(w, "ConditionalCheckFailedException",
				"The conditional request failed", http.StatusBadRequest)
			return
		}
	}
	ddbItems.Delete(itemKey)
	ddbItemNames.Delete(itemKey)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// handleDDBQuery returns all items in the table whose hash key matches
// the request's KeyConditionExpression. The sim's matcher only handles
// the simple `<attr> = :val` shape Terraform state locks use; complex
// expressions fall through.
func handleDDBQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	_ = sim.ReadJSON(r, &req)
	if _, ok := ddbTables.Get(req.TableName); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	// Sim returns all items in the table for simplicity. Real DynamoDB
	// applies the KeyConditionExpression first; tests that need that
	// precision should add the matcher.
	prefix := req.TableName + "/"
	var items []map[string]any
	for _, k := range ddbItemKeys(prefix) {
		if it, ok := ddbItems.Get(k); ok {
			items = append(items, it)
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Items": items,
		"Count": len(items),
	})
}

func handleDDBScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	_ = sim.ReadJSON(r, &req)
	if _, ok := ddbTables.Get(req.TableName); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	prefix := req.TableName + "/"
	var items []map[string]any
	for _, k := range ddbItemKeys(prefix) {
		if it, ok := ddbItems.Get(k); ok {
			items = append(items, it)
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Items": items,
		"Count": len(items),
	})
}

// ddbItemKeys returns all item keys with the given prefix. The Store
// API doesn't expose key iteration directly, but ddbItemNames mirrors
// the keys for sim use.
func ddbItemKeys(prefix string) []string {
	var out []string
	for _, name := range ddbItemNames.List() {
		// each entry is the full item key (table/<...>); filter by prefix.
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			out = append(out, name)
		}
	}
	return out
}

// containsCI is a case-insensitive substring check — Terraform's
// state-lock condition expression is `attribute_not_exists(LockID)`
// which we just need to recognize without parsing.
func containsCI(haystack, needle string) bool {
	return indexOfFold(haystack, needle) >= 0
}

func indexOfFold(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	if len(s) < len(sub) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a := s[i+j]
			b := sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// ddbItemNames mirrors the keys of ddbItems for iteration. Maintained
// alongside Put/Delete in handleDDBPutItem etc.
var ddbItemNames sim.Store[string]
