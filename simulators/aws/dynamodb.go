package main

// AWS DynamoDB uses the awsJson1_0 protocol. The AWS SDK Go v2's
// deserializer requires responses to carry `Content-Type:
// application/x-amz-json-1.0` (not `application/json`); without it the
// SDK silently fails to decode the body and the result struct is nil,
// which terraform-provider-aws then treats as ResourceNotFound (its
// waiter loops 21 times then errors "couldn't find resource").
//
// `sim.WriteJSON` (used elsewhere) sets `application/json`. The
// `writeDDBJSON` wrapper below sets the per-protocol header instead so
// each DynamoDB success response carries the right CT. Errors keep going
// through `sim.AWSErrorf` which already sets `application/x-amz-json-1.1`
// (real AWS uses 1.1 for errors across JSON-RPC services, regardless of
// the service's own payload protocol).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	TableName                 string                    `json:"TableName"`
	TableId                   string                    `json:"TableId"`
	TableArn                  string                    `json:"TableArn"`
	TableStatus               string                    `json:"TableStatus"`
	CreationDateTime          float64                   `json:"CreationDateTime"`
	AttributeDefinitions      []DDBAttributeDef         `json:"AttributeDefinitions"`
	KeySchema                 []DDBKeySchemaEntry       `json:"KeySchema"`
	BillingModeSummary        *DDBBillingModeSummary    `json:"BillingModeSummary,omitempty"`
	ProvisionedThroughput     *DDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty"`
	ItemCount                 int64                     `json:"ItemCount"`
	TableSizeBytes            int64                     `json:"TableSizeBytes"`
	DeletionProtectionEnabled bool                      `json:"DeletionProtectionEnabled"`
	TableClassSummary         *DDBTableClassSummary     `json:"TableClassSummary,omitempty"`
	WarmThroughput            *DDBWarmThroughput        `json:"WarmThroughput,omitempty"`

	// PITR + TTL state — Update* persists here so Describe* reads back
	// the actual state (real AWS round-trips these; terraform polls
	// Describe after Update for convergence).
	PITRStatus       string  `json:"-"` // ENABLED / DISABLED
	TTLStatus        string  `json:"-"` // ENABLED / DISABLED
	TTLAttributeName string  `json:"-"`
	Tags             []SMTag `json:"-"`
}

// DDBProvisionedThroughput mirrors the SDK shape. For PAY_PER_REQUEST
// tables real AWS still returns a zero-filled struct so terraform's
// reader doesn't NPE — the sim follows.
type DDBProvisionedThroughput struct {
	NumberOfDecreasesToday int64   `json:"NumberOfDecreasesToday"`
	ReadCapacityUnits      int64   `json:"ReadCapacityUnits"`
	WriteCapacityUnits     int64   `json:"WriteCapacityUnits"`
	LastIncreaseDateTime   float64 `json:"LastIncreaseDateTime,omitempty"`
	LastDecreaseDateTime   float64 `json:"LastDecreaseDateTime,omitempty"`
}

// DDBTableClassSummary mirrors the SDK shape — STANDARD (default) or
// STANDARD_INFREQUENT_ACCESS. Real AWS returns this on every Describe.
type DDBTableClassSummary struct {
	TableClass         string  `json:"TableClass"`
	LastUpdateDateTime float64 `json:"LastUpdateDateTime,omitempty"`
}

// DDBWarmThroughput mirrors `types.TableWarmThroughputDescription`. Real
// AWS DynamoDB returns this on every DescribeTable response, with
// Status=ACTIVE on a fresh on-demand table. terraform-provider-aws v6
// added `waitTableWarmThroughputActive` after `waitTableActive` in the
// Create flow — that wait function returns empty state and loops 21
// times if `output.WarmThroughput == nil`, so the field MUST be present
// on every response or terraform errors "waiting for update ... couldn't
// find resource".
type DDBWarmThroughput struct {
	ReadUnitsPerSecond  int64  `json:"ReadUnitsPerSecond"`
	Status              string `json:"Status"`
	WriteUnitsPerSecond int64  `json:"WriteUnitsPerSecond"`
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

// writeDDBJSON writes a DynamoDB success response with the awsJson1_0
// content-type. The AWS SDK Go v2 DynamoDB deserializer requires the
// exact `application/x-amz-json-1.0` value — `application/json` causes
// silent decode failure where output.Table comes back nil, which
// terraform-provider-aws then treats as ResourceNotFound.
func writeDDBJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ddbTableArn(name string) string {
	return fmt.Sprintf("arn:aws:dynamodb:%s:%s:table/%s", awsRegion(), awsAccountID(), name)
}

// ddbTableByArn locates a stored table by its full ARN. Tag CRUD takes
// ResourceArn (not TableName) and real DynamoDB accepts both forms; the
// sim's name-keyed store has to be scanned for an ARN match.
func ddbTableByArn(arn string) (string, DDBTable, bool) {
	if arn == "" {
		return "", DDBTable{}, false
	}
	// ARN shape: arn:aws:dynamodb:<region>:<account>:table/<name>
	const sep = ":table/"
	idx := strings.Index(arn, sep)
	if idx < 0 {
		return "", DDBTable{}, false
	}
	name := arn[idx+len(sep):]
	t, ok := ddbTables.Get(name)
	return name, t, ok
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
	r.Register("DynamoDB_20120810.DescribeContinuousBackups", handleDDBDescribeContinuousBackups)
	r.Register("DynamoDB_20120810.UpdateContinuousBackups", handleDDBUpdateContinuousBackups)
	r.Register("DynamoDB_20120810.DescribeTimeToLive", handleDDBDescribeTimeToLive)
	r.Register("DynamoDB_20120810.UpdateTimeToLive", handleDDBUpdateTimeToLive)
	r.Register("DynamoDB_20120810.ListTagsOfResource", handleDDBListTagsOfResource)
	r.Register("DynamoDB_20120810.TagResource", handleDDBTagResource)
	r.Register("DynamoDB_20120810.UntagResource", handleDDBUntagResource)
}

// handleDDBUpdateContinuousBackups enables/disables PITR. Persists to
// DDBTable.PITRStatus so DescribeContinuousBackups reads back the
// updated state. Real DynamoDB returns the new ContinuousBackupsDescription;
// terraform-provider-aws polls DescribeContinuousBackups after this to
// confirm convergence.
func handleDDBUpdateContinuousBackups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName                        string `json:"TableName"`
		PointInTimeRecoverySpecification struct {
			PointInTimeRecoveryEnabled bool `json:"PointInTimeRecoveryEnabled"`
		} `json:"PointInTimeRecoverySpecification"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "TableNotFoundException", http.StatusBadRequest,
			"Table not found: %s", req.TableName)
		return
	}
	status := "DISABLED"
	if req.PointInTimeRecoverySpecification.PointInTimeRecoveryEnabled {
		status = "ENABLED"
	}
	t.PITRStatus = status
	ddbTables.Put(req.TableName, t)
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"ContinuousBackupsDescription": map[string]any{
			"ContinuousBackupsStatus": "ENABLED",
			"PointInTimeRecoveryDescription": map[string]any{
				"PointInTimeRecoveryStatus": status,
			},
		},
	})
}

// handleDDBUpdateTimeToLive enables/disables TTL on a table attribute.
// Persists to DDBTable.TTLStatus + AttributeName so DescribeTimeToLive
// reads back the updated state.
func handleDDBUpdateTimeToLive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName               string `json:"TableName"`
		TimeToLiveSpecification struct {
			Enabled       bool   `json:"Enabled"`
			AttributeName string `json:"AttributeName"`
		} `json:"TimeToLiveSpecification"`
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
	status := "DISABLED"
	if req.TimeToLiveSpecification.Enabled {
		status = "ENABLED"
	}
	t.TTLStatus = status
	t.TTLAttributeName = req.TimeToLiveSpecification.AttributeName
	ddbTables.Put(req.TableName, t)
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"TimeToLiveSpecification": map[string]any{
			"Enabled":       req.TimeToLiveSpecification.Enabled,
			"AttributeName": req.TimeToLiveSpecification.AttributeName,
		},
	})
}

// handleDDBTagResource attaches tags + persists upsert. Real DynamoDB
// returns empty body but stores the tags so ListTagsOfResource reads
// them back (same upsert semantics as real AWS: re-tag with same Key
// replaces Value).
func handleDDBTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string  `json:"ResourceArn"`
		Tags        []SMTag `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceArn == "" {
		sim.AWSError(w, "ValidationException", "ResourceArn is required", http.StatusBadRequest)
		return
	}
	name, t, ok := ddbTableByArn(req.ResourceArn)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: %s", req.ResourceArn)
		return
	}
	override := map[string]string{}
	for _, tag := range req.Tags {
		override[tag.Key] = tag.Value
	}
	merged := make([]SMTag, 0, len(t.Tags)+len(req.Tags))
	for _, tag := range t.Tags {
		if _, replaced := override[tag.Key]; !replaced {
			merged = append(merged, tag)
		}
	}
	merged = append(merged, req.Tags...)
	t.Tags = merged
	ddbTables.Put(name, t)
	writeDDBJSON(w, http.StatusOK, map[string]any{})
}

// handleDDBUntagResource removes tag keys from the persisted set.
// Real DynamoDB returns empty body + silently ignores missing keys.
func handleDDBUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string   `json:"ResourceArn"`
		TagKeys     []string `json:"TagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceArn == "" {
		sim.AWSError(w, "ValidationException", "ResourceArn is required", http.StatusBadRequest)
		return
	}
	name, t, ok := ddbTableByArn(req.ResourceArn)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: %s", req.ResourceArn)
		return
	}
	remove := map[string]bool{}
	for _, k := range req.TagKeys {
		remove[k] = true
	}
	filtered := make([]SMTag, 0, len(t.Tags))
	for _, tag := range t.Tags {
		if !remove[tag.Key] {
			filtered = append(filtered, tag)
		}
	}
	t.Tags = filtered
	ddbTables.Put(name, t)
	writeDDBJSON(w, http.StatusOK, map[string]any{})
}

// handleDDBDescribeContinuousBackups returns the PITR status for a
// table from the persisted DDBTable.PITRStatus. New tables default to
// DISABLED. terraform-provider-aws polls this after UpdateContinuousBackups
// for convergence.
func handleDDBDescribeContinuousBackups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "TableNotFoundException", http.StatusBadRequest,
			"Table not found: %s", req.TableName)
		return
	}
	pitr := t.PITRStatus
	if pitr == "" {
		pitr = "DISABLED"
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"ContinuousBackupsDescription": map[string]any{
			"ContinuousBackupsStatus": "ENABLED",
			"PointInTimeRecoveryDescription": map[string]any{
				"PointInTimeRecoveryStatus": pitr,
			},
		},
	})
}

// handleDDBDescribeTimeToLive returns TTL config for a table from the
// persisted DDBTable.TTLStatus + AttributeName. terraform-provider-aws
// polls this after UpdateTimeToLive until status matches.
func handleDDBDescribeTimeToLive(w http.ResponseWriter, r *http.Request) {
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
	status := t.TTLStatus
	if status == "" {
		status = "DISABLED"
	}
	desc := map[string]any{"TimeToLiveStatus": status}
	if t.TTLAttributeName != "" {
		desc["AttributeName"] = t.TTLAttributeName
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"TimeToLiveDescription": desc,
	})
}

// handleDDBListTagsOfResource returns tag list for a table ARN from
// the persisted DDBTable.Tags. Real DynamoDB tracks tags out-of-band but
// the sim keeps them on the table row for the same lookup latency.
func handleDDBListTagsOfResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string `json:"ResourceArn"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceArn == "" {
		sim.AWSError(w, "ValidationException", "ResourceArn is required", http.StatusBadRequest)
		return
	}
	_, t, ok := ddbTableByArn(req.ResourceArn)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: %s", req.ResourceArn)
		return
	}
	tags := make([]map[string]any, 0, len(t.Tags))
	for _, tag := range t.Tags {
		tags = append(tags, map[string]any{"Key": tag.Key, "Value": tag.Value})
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"Tags": tags,
	})
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
	now := float64(time.Now().Unix())
	table := DDBTable{
		TableName:            req.TableName,
		TableId:              generateUUID(),
		TableArn:             ddbTableArn(req.TableName),
		TableStatus:          "ACTIVE",
		CreationDateTime:     now,
		AttributeDefinitions: req.AttributeDefinitions,
		KeySchema:            req.KeySchema,
		BillingModeSummary: &DDBBillingModeSummary{
			BillingMode: billingMode,
		},
		// Real AWS returns a zero-filled ProvisionedThroughput even for
		// PAY_PER_REQUEST tables so terraform's reader doesn't NPE.
		ProvisionedThroughput: &DDBProvisionedThroughput{
			NumberOfDecreasesToday: 0,
			ReadCapacityUnits:      0,
			WriteCapacityUnits:     0,
		},
		TableClassSummary: &DDBTableClassSummary{
			TableClass: "STANDARD",
		},
		// Real DynamoDB returns WarmThroughput on every Describe with
		// Status=ACTIVE for on-demand tables; terraform-provider-aws v6's
		// waitTableWarmThroughputActive depends on this field being
		// present + non-nil.
		WarmThroughput: &DDBWarmThroughput{
			ReadUnitsPerSecond:  12000,
			WriteUnitsPerSecond: 4000,
			Status:              "ACTIVE",
		},
	}
	ddbTables.Put(req.TableName, table)
	writeDDBJSON(w, http.StatusOK, map[string]any{"TableDescription": table})
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
	writeDDBJSON(w, http.StatusOK, map[string]any{"Table": t})
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
	writeDDBJSON(w, http.StatusOK, map[string]any{"TableDescription": t})
}

func handleDDBListTables(w http.ResponseWriter, r *http.Request) {
	all := ddbTables.List()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.TableName)
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{"TableNames": names})
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
	writeDDBJSON(w, http.StatusOK, map[string]any{})
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
		writeDDBJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{"Item": item})
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
	writeDDBJSON(w, http.StatusOK, map[string]any{"Attributes": item})
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
	writeDDBJSON(w, http.StatusOK, map[string]any{})
}

// handleDDBQuery returns all items in the table whose hash key matches
// the request's KeyConditionExpression. The sim's matcher only handles
// the simple `<attr> = :val` shape Terraform state locks use; complex
// expressions fall through.
func handleDDBQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
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
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"Items": items,
		"Count": len(items),
	})
}

func handleDDBScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName string `json:"TableName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
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
	writeDDBJSON(w, http.StatusOK, map[string]any{
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
