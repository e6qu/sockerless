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
	"sort"
	"strconv"
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
	GlobalSecondaryIndexes    []DDBGlobalSecondaryIndex `json:"GlobalSecondaryIndexes,omitempty"`
	LocalSecondaryIndexes     []DDBLocalSecondaryIndex  `json:"LocalSecondaryIndexes,omitempty"`
	BillingModeSummary        *DDBBillingModeSummary    `json:"BillingModeSummary,omitempty"`
	ProvisionedThroughput     *DDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty"`
	ItemCount                 int64                     `json:"ItemCount"`
	TableSizeBytes            int64                     `json:"TableSizeBytes"`
	DeletionProtectionEnabled bool                      `json:"DeletionProtectionEnabled"`
	TableClassSummary         *DDBTableClassSummary     `json:"TableClassSummary,omitempty"`
	WarmThroughput            *DDBWarmThroughput        `json:"WarmThroughput,omitempty"`
	SSEDescription            *DDBSSEDescription        `json:"SSEDescription,omitempty"`

	// PITR + TTL state — Update* persists here so Describe* reads back
	// the actual state (real AWS round-trips these; terraform polls
	// Describe after Update for convergence).
	PITRStatus       string  `json:"-"` // ENABLED / DISABLED
	TTLStatus        string  `json:"-"` // ENABLED / DISABLED
	TTLAttributeName string  `json:"-"`
	Tags             []SMTag `json:"-"`
}

// DDBSSEDescription mirrors the SDK SSEDescription returned by DescribeTable for
// an SSE-enabled table.
type DDBSSEDescription struct {
	Status          string `json:"Status"`
	SSEType         string `json:"SSEType"`
	KMSMasterKeyArn string `json:"KMSMasterKeyArn,omitempty"`
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

// DDBProjection is a secondary index's attribute projection.
type DDBProjection struct {
	ProjectionType   string   `json:"ProjectionType"` // ALL / KEYS_ONLY / INCLUDE
	NonKeyAttributes []string `json:"NonKeyAttributes,omitempty"`
}

// DDBGlobalSecondaryIndex mirrors the GSI shape. The CreateTable request
// carries IndexName/KeySchema/Projection/ProvisionedThroughput; the
// Create/Describe responses additionally carry IndexStatus (ACTIVE),
// IndexArn, ItemCount, and IndexSizeBytes — terraform-provider-aws waits for
// IndexStatus==ACTIVE on every GSI before the table converges.
type DDBGlobalSecondaryIndex struct {
	IndexName             string                    `json:"IndexName"`
	KeySchema             []DDBKeySchemaEntry       `json:"KeySchema"`
	Projection            *DDBProjection            `json:"Projection,omitempty"`
	IndexStatus           string                    `json:"IndexStatus,omitempty"`
	IndexArn              string                    `json:"IndexArn,omitempty"`
	ItemCount             int64                     `json:"ItemCount"`
	IndexSizeBytes        int64                     `json:"IndexSizeBytes"`
	ProvisionedThroughput *DDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty"`
	WarmThroughput        *DDBWarmThroughput        `json:"WarmThroughput,omitempty"`
}

// DDBLocalSecondaryIndex mirrors the LSI shape. LSIs are created with the
// table and have no independent status (no IndexStatus field).
type DDBLocalSecondaryIndex struct {
	IndexName      string              `json:"IndexName"`
	KeySchema      []DDBKeySchemaEntry `json:"KeySchema"`
	Projection     *DDBProjection      `json:"Projection,omitempty"`
	IndexArn       string              `json:"IndexArn,omitempty"`
	ItemCount      int64               `json:"ItemCount"`
	IndexSizeBytes int64               `json:"IndexSizeBytes"`
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

func ddbIndexArn(table, index string) string {
	return fmt.Sprintf("arn:aws:dynamodb:%s:%s:table/%s/index/%s", awsRegion(), awsAccountID(), table, index)
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
	r.Register("DynamoDB_20120810.UpdateTable", handleDDBUpdateTable)
	r.Register("DynamoDB_20120810.DeleteTable", handleDDBDeleteTable)
	r.Register("DynamoDB_20120810.ListTables", handleDDBListTables)
	r.Register("DynamoDB_20120810.PutItem", handleDDBPutItem)
	r.Register("DynamoDB_20120810.GetItem", handleDDBGetItem)
	r.Register("DynamoDB_20120810.UpdateItem", handleDDBUpdateItem)
	r.Register("DynamoDB_20120810.DeleteItem", handleDDBDeleteItem)
	r.Register("DynamoDB_20120810.Query", handleDDBQuery)
	r.Register("DynamoDB_20120810.Scan", handleDDBScan)
	r.Register("DynamoDB_20120810.BatchWriteItem", handleDDBBatchWriteItem)
	r.Register("DynamoDB_20120810.BatchGetItem", handleDDBBatchGetItem)
	r.Register("DynamoDB_20120810.TransactWriteItems", handleDDBTransactWriteItems)
	r.Register("DynamoDB_20120810.TransactGetItems", handleDDBTransactGetItems)
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
		TableName              string                    `json:"TableName"`
		AttributeDefinitions   []DDBAttributeDef         `json:"AttributeDefinitions"`
		KeySchema              []DDBKeySchemaEntry       `json:"KeySchema"`
		BillingMode            string                    `json:"BillingMode"`
		GlobalSecondaryIndexes []DDBGlobalSecondaryIndex `json:"GlobalSecondaryIndexes"`
		LocalSecondaryIndexes  []DDBLocalSecondaryIndex  `json:"LocalSecondaryIndexes"`
		SSESpecification       *struct {
			Enabled        bool   `json:"Enabled"`
			SSEType        string `json:"SSEType"`
			KMSMasterKeyId string `json:"KMSMasterKeyId"`
		} `json:"SSESpecification"`
		Tags []SMTag `json:"Tags"`
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

	// Model secondary indexes as immediately ACTIVE. terraform-provider-aws
	// waits for every GSI's IndexStatus to reach ACTIVE before the table
	// converges; pre-fix the indexes were dropped entirely (returned null).
	gsis := make([]DDBGlobalSecondaryIndex, 0, len(req.GlobalSecondaryIndexes))
	for _, g := range req.GlobalSecondaryIndexes {
		gsis = append(gsis, ddbActivateGSI(req.TableName, g))
	}
	lsis := make([]DDBLocalSecondaryIndex, 0, len(req.LocalSecondaryIndexes))
	for _, l := range req.LocalSecondaryIndexes {
		l.IndexArn = ddbIndexArn(req.TableName, l.IndexName)
		lsis = append(lsis, l)
	}

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
	if len(gsis) > 0 {
		table.GlobalSecondaryIndexes = gsis
	}
	if len(lsis) > 0 {
		table.LocalSecondaryIndexes = lsis
	}
	// Server-side encryption: once enabled, real DynamoDB reports the full
	// descriptor (Status ENABLED) on every Describe. SSEType defaults to KMS
	// (a customer/AWS-managed key) when omitted.
	if req.SSESpecification != nil && req.SSESpecification.Enabled {
		sseType := req.SSESpecification.SSEType
		if sseType == "" {
			sseType = "KMS"
		}
		table.SSEDescription = &DDBSSEDescription{
			Status:          "ENABLED",
			SSEType:         sseType,
			KMSMasterKeyArn: req.SSESpecification.KMSMasterKeyId,
		}
	}
	// Tags set at create time round-trip through ListTagsOfResource — real
	// DynamoDB accepts Tags on CreateTable; dropping them makes every plan
	// re-add them.
	table.Tags = req.Tags
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

// ddbActivateGSI fills the stored/response fields of a GSI so it reports as
// immediately ACTIVE (the sim models index builds as instantaneous).
func ddbActivateGSI(tableName string, g DDBGlobalSecondaryIndex) DDBGlobalSecondaryIndex {
	g.IndexStatus = "ACTIVE"
	g.IndexArn = ddbIndexArn(tableName, g.IndexName)
	if g.ProvisionedThroughput == nil {
		g.ProvisionedThroughput = &DDBProvisionedThroughput{}
	}
	g.WarmThroughput = &DDBWarmThroughput{
		ReadUnitsPerSecond:  12000,
		WriteUnitsPerSecond: 4000,
		Status:              "ACTIVE",
	}
	return g
}

func ddbMergeAttributeDefs(existing, incoming []DDBAttributeDef) []DDBAttributeDef {
	seen := map[string]bool{}
	out := make([]DDBAttributeDef, 0, len(existing)+len(incoming))
	for _, a := range existing {
		if !seen[a.AttributeName] {
			seen[a.AttributeName] = true
			out = append(out, a)
		}
	}
	for _, a := range incoming {
		if !seen[a.AttributeName] {
			seen[a.AttributeName] = true
			out = append(out, a)
		}
	}
	return out
}

// handleDDBUpdateTable applies GSI create/update/delete, throughput, billing
// mode, and deletion-protection changes. terraform-provider-aws manages the GSI
// lifecycle after table creation via UpdateTable's GlobalSecondaryIndexUpdates,
// then polls DescribeTable until each new GSI's IndexStatus is ACTIVE.
func handleDDBUpdateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName                 string                    `json:"TableName"`
		AttributeDefinitions      []DDBAttributeDef         `json:"AttributeDefinitions"`
		BillingMode               string                    `json:"BillingMode"`
		DeletionProtectionEnabled *bool                     `json:"DeletionProtectionEnabled"`
		ProvisionedThroughput     *DDBProvisionedThroughput `json:"ProvisionedThroughput"`

		GlobalSecondaryIndexUpdates []struct {
			Create *DDBGlobalSecondaryIndex `json:"Create"`
			Update *struct {
				IndexName             string                    `json:"IndexName"`
				ProvisionedThroughput *DDBProvisionedThroughput `json:"ProvisionedThroughput"`
			} `json:"Update"`
			Delete *struct {
				IndexName string `json:"IndexName"`
			} `json:"Delete"`
		} `json:"GlobalSecondaryIndexUpdates"`
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
	if len(req.AttributeDefinitions) > 0 {
		t.AttributeDefinitions = ddbMergeAttributeDefs(t.AttributeDefinitions, req.AttributeDefinitions)
	}
	if req.BillingMode != "" {
		if t.BillingModeSummary == nil {
			t.BillingModeSummary = &DDBBillingModeSummary{}
		}
		t.BillingModeSummary.BillingMode = req.BillingMode
	}
	if req.ProvisionedThroughput != nil {
		t.ProvisionedThroughput = req.ProvisionedThroughput
	}
	if req.DeletionProtectionEnabled != nil {
		t.DeletionProtectionEnabled = *req.DeletionProtectionEnabled
	}
	for _, upd := range req.GlobalSecondaryIndexUpdates {
		switch {
		case upd.Create != nil:
			t.GlobalSecondaryIndexes = append(t.GlobalSecondaryIndexes, ddbActivateGSI(req.TableName, *upd.Create))
		case upd.Delete != nil:
			kept := t.GlobalSecondaryIndexes[:0:0]
			for _, g := range t.GlobalSecondaryIndexes {
				if g.IndexName != upd.Delete.IndexName {
					kept = append(kept, g)
				}
			}
			t.GlobalSecondaryIndexes = kept
		case upd.Update != nil && upd.Update.ProvisionedThroughput != nil:
			for i := range t.GlobalSecondaryIndexes {
				if t.GlobalSecondaryIndexes[i].IndexName == upd.Update.IndexName {
					t.GlobalSecondaryIndexes[i].ProvisionedThroughput = upd.Update.ProvisionedThroughput
				}
			}
		}
	}
	ddbTables.Put(req.TableName, t)
	writeDDBJSON(w, http.StatusOK, map[string]any{"TableDescription": t})
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
	var req struct {
		ExclusiveStartTableName string `json:"ExclusiveStartTableName"`
		Limit                   int    `json:"Limit"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValue", "Invalid request body", http.StatusBadRequest)
		return
	}
	all := ddbTables.List()
	sortBy(all, func(t DDBTable) string { return t.TableName })

	// ExclusiveStartTableName is a name-based cursor; convert to offset token.
	token := ""
	if req.ExclusiveStartTableName != "" {
		for i, t := range all {
			if t.TableName == req.ExclusiveStartTableName {
				token = strconv.Itoa(i + 1)
				break
			}
		}
	}
	page, next := awsPage(all, token, req.Limit, 100)
	names := make([]string, 0, len(page))
	for _, t := range page {
		names = append(names, t.TableName)
	}
	out := map[string]any{"TableNames": names}
	if next != "" {
		// Convert token back to a table name for LastEvaluatedTableName.
		idx, _ := strconv.Atoi(next)
		if idx > 0 && idx <= len(all) {
			out["LastEvaluatedTableName"] = all[idx-1].TableName
		}
	}
	writeDDBJSON(w, http.StatusOK, out)
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
		TableName                 string            `json:"TableName"`
		Item                      map[string]any    `json:"Item"`
		ConditionExpression       string            `json:"ConditionExpression"`
		ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames,omitempty"`
		ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues,omitempty"`
		ReturnValues              string            `json:"ReturnValues,omitempty"`
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
	old, exists := ddbItems.Get(itemKey)

	// Atomically evaluate the ConditionExpression (e.g. terraform's state-lock
	// "attribute_not_exists(LockID)") before writing.
	if req.ConditionExpression != "" &&
		!ddbEvalCondition(old, exists, req.ConditionExpression, req.ExpressionAttributeNames, req.ExpressionAttributeValues) {
		sim.AWSError(w, "ConditionalCheckFailedException",
			"The conditional request failed", http.StatusBadRequest)
		return
	}
	ddbItems.Put(itemKey, req.Item)
	ddbItemNames.Put(itemKey, itemKey)
	resp := map[string]any{}
	if req.ReturnValues == "ALL_OLD" && exists {
		resp["Attributes"] = old
	}
	writeDDBJSON(w, http.StatusOK, resp)
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
		TableName                 string            `json:"TableName"`
		Key                       map[string]any    `json:"Key"`
		ConditionExpression       string            `json:"ConditionExpression"`
		ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames"`
		ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues"`
		ReturnValues              string            `json:"ReturnValues"`
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
	oldItem, existed := ddbItems.Get(itemKey)
	if req.ConditionExpression != "" &&
		!ddbEvalCondition(oldItem, existed, req.ConditionExpression, req.ExpressionAttributeNames, req.ExpressionAttributeValues) {
		sim.AWSError(w, "ConditionalCheckFailedException",
			"The conditional request failed", http.StatusBadRequest)
		return
	}
	ddbItems.Delete(itemKey)
	ddbItemNames.Delete(itemKey)
	if strings.EqualFold(req.ReturnValues, "ALL_OLD") && existed {
		writeDDBJSON(w, http.StatusOK, map[string]any{"Attributes": oldItem})
		return
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{})
}

// handleDDBQuery returns items whose primary-key attributes match the
// request's KeyConditionExpression. The implemented expression subset is
// the DynamoDB equality form used by SDK/CLI/Terraform clients:
// `<hash> = :value` plus optional `AND <range> = :value`.
func handleDDBQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName                 string            `json:"TableName"`
		IndexName                 string            `json:"IndexName"`
		KeyConditionExpression    string            `json:"KeyConditionExpression"`
		ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames"`
		ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues"`
		Limit                     int               `json:"Limit"`
		ExclusiveStartKey         map[string]any    `json:"ExclusiveStartKey"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	// IndexName selects a GSI/LSI; the KeyConditionExpression then matches that
	// index's key attributes. The matcher is generic over item attributes, so a
	// GSI query needs no special handling beyond rejecting an unknown index.
	if req.IndexName != "" && !ddbHasIndex(t, req.IndexName) {
		sim.AWSErrorf(w, "ValidationException", http.StatusBadRequest,
			"The table does not have the specified index: %s", req.IndexName)
		return
	}
	prefix := req.TableName + "/"
	keys := ddbItemKeys(prefix)
	sort.Strings(keys)

	// Advance past ExclusiveStartKey if provided.
	startKey := ddbItemKey(t, req.ExclusiveStartKey)
	startIdx := 0
	if startKey != prefix && startKey != t.TableName+"/" {
		for i, k := range keys {
			if k == startKey {
				startIdx = i + 1
				break
			}
		}
	}

	var items []map[string]any
	for _, k := range keys[startIdx:] {
		if it, ok2 := ddbItems.Get(k); ok2 {
			if ddbMatchesExpression(t, it, req.KeyConditionExpression, req.ExpressionAttributeNames, req.ExpressionAttributeValues) {
				items = append(items, it)
			}
		}
		if req.Limit > 0 && len(items) >= req.Limit {
			break
		}
	}
	if items == nil {
		items = []map[string]any{}
	}

	out := map[string]any{"Items": items, "Count": len(items), "ScannedCount": len(items)}
	// Emit LastEvaluatedKey if we hit the Limit and more items may exist.
	if req.Limit > 0 && len(items) == req.Limit {
		last := items[len(items)-1]
		out["LastEvaluatedKey"] = ddbExtractKey(t, last)
	}
	writeDDBJSON(w, http.StatusOK, out)
}

// ddbHasIndex reports whether the table has a GSI or LSI with the given name.
func ddbHasIndex(t DDBTable, name string) bool {
	for _, g := range t.GlobalSecondaryIndexes {
		if g.IndexName == name {
			return true
		}
	}
	for _, l := range t.LocalSecondaryIndexes {
		if l.IndexName == name {
			return true
		}
	}
	return false
}

func handleDDBScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName                 string            `json:"TableName"`
		IndexName                 string            `json:"IndexName"`
		FilterExpression          string            `json:"FilterExpression"`
		ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames"`
		ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues"`
		Limit                     int               `json:"Limit"`
		ExclusiveStartKey         map[string]any    `json:"ExclusiveStartKey"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	t, ok := ddbTables.Get(req.TableName)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Requested resource not found: Table: %s not found", req.TableName)
		return
	}
	if req.IndexName != "" && !ddbHasIndex(t, req.IndexName) {
		sim.AWSErrorf(w, "ValidationException", http.StatusBadRequest,
			"The table does not have the specified index: %s", req.IndexName)
		return
	}
	prefix := req.TableName + "/"
	keys := ddbItemKeys(prefix)
	sort.Strings(keys)

	startKey := ddbItemKey(t, req.ExclusiveStartKey)
	startIdx := 0
	if startKey != prefix && startKey != t.TableName+"/" {
		for i, k := range keys {
			if k == startKey {
				startIdx = i + 1
				break
			}
		}
	}

	var items []map[string]any
	scanned := 0
	for _, k := range keys[startIdx:] {
		if it, ok2 := ddbItems.Get(k); ok2 {
			scanned++
			if ddbMatchesExpression(DDBTable{}, it, req.FilterExpression, req.ExpressionAttributeNames, req.ExpressionAttributeValues) {
				items = append(items, it)
			}
		}
		if req.Limit > 0 && len(items) >= req.Limit {
			break
		}
	}
	if items == nil {
		items = []map[string]any{}
	}

	out := map[string]any{"Items": items, "Count": len(items), "ScannedCount": scanned}
	if req.Limit > 0 && len(items) == req.Limit {
		last := items[len(items)-1]
		out["LastEvaluatedKey"] = ddbExtractKey(t, last)
	}
	writeDDBJSON(w, http.StatusOK, out)
}

// ddbEvalCondition reports whether a DynamoDB ConditionExpression holds for an
// item. `exists` is whether the item is currently present. Supports the common
// subset: attribute_exists / attribute_not_exists, begins_with, and the
// comparison operators (=, <>, <, <=, >, >=), combined with AND. An empty
// expression always holds.
func ddbEvalCondition(item map[string]any, exists bool, expr string, names map[string]string, values map[string]any) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	for _, raw := range splitTopLevelAnd(expr) {
		clause := strings.TrimSpace(raw)
		if !ddbEvalConditionClause(item, exists, clause, names, values) {
			return false
		}
	}
	return true
}

func splitTopLevelAnd(expr string) []string {
	// AND is the only conjunction the sim models; split case-insensitively.
	lower := strings.ToLower(expr)
	var parts []string
	last := 0
	for {
		idx := strings.Index(lower[last:], " and ")
		if idx < 0 {
			parts = append(parts, expr[last:])
			break
		}
		parts = append(parts, expr[last:last+idx])
		last += idx + len(" and ")
	}
	return parts
}

func ddbEvalConditionClause(item map[string]any, exists bool, clause string, names map[string]string, values map[string]any) bool {
	clause = strings.TrimSpace(clause)
	if rest, ok := ddbFuncArg(clause, "attribute_not_exists"); ok {
		attr := ddbResolveAttrName(rest, names)
		return !exists || item[attr] == nil
	}
	if rest, ok := ddbFuncArg(clause, "attribute_exists"); ok {
		attr := ddbResolveAttrName(rest, names)
		return exists && item[attr] != nil
	}
	if rest, ok := ddbFuncArg(clause, "begins_with"); ok {
		args := strings.SplitN(rest, ",", 2)
		if len(args) != 2 {
			return false
		}
		attr := ddbResolveAttrName(strings.TrimSpace(args[0]), names)
		want := ddbScalarString(values[strings.TrimSpace(args[1])])
		return strings.HasPrefix(ddbScalarString(item[attr]), want)
	}
	// Comparison operators, longest-first so "<=" / ">=" / "<>" win over "<"/">".
	for _, op := range []string{"<=", ">=", "<>", "=", "<", ">"} {
		if left, right, found := strings.Cut(clause, op); found {
			attr := ddbResolveAttrName(strings.TrimSpace(left), names)
			want, ok := values[strings.TrimSpace(right)]
			if !ok {
				return false
			}
			return ddbCompare(item[attr], want, op)
		}
	}
	return false
}

// ddbFuncArg returns the single argument of fn(arg) if clause matches.
func ddbFuncArg(clause, fn string) (string, bool) {
	clause = strings.TrimSpace(clause)
	if !strings.HasPrefix(strings.ToLower(clause), fn+"(") || !strings.HasSuffix(clause, ")") {
		return "", false
	}
	return strings.TrimSpace(clause[len(fn)+1 : len(clause)-1]), true
}

func ddbScalarString(v any) string {
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"S", "N", "B"} {
			if s, ok := m[k]; ok {
				return fmt.Sprint(s)
			}
		}
	}
	return fmt.Sprint(v)
}

func ddbCompare(a, b any, op string) bool {
	if op == "=" {
		return ddbAttrValuesEqual(a, b)
	}
	if op == "<>" {
		return !ddbAttrValuesEqual(a, b)
	}
	// Numeric comparison when both sides are N; lexicographic otherwise.
	as, bs := ddbScalarString(a), ddbScalarString(b)
	if af, aerr := strconv.ParseFloat(as, 64); aerr == nil {
		if bf, berr := strconv.ParseFloat(bs, 64); berr == nil {
			switch op {
			case "<":
				return af < bf
			case "<=":
				return af <= bf
			case ">":
				return af > bf
			case ">=":
				return af >= bf
			}
		}
	}
	switch op {
	case "<":
		return as < bs
	case "<=":
		return as <= bs
	case ">":
		return as > bs
	case ">=":
		return as >= bs
	}
	return false
}

func handleDDBBatchWriteItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RequestItems map[string][]struct {
			PutRequest *struct {
				Item map[string]any `json:"Item"`
			} `json:"PutRequest"`
			DeleteRequest *struct {
				Key map[string]any `json:"Key"`
			} `json:"DeleteRequest"`
		} `json:"RequestItems"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()
	for tableName, ops := range req.RequestItems {
		t, ok := ddbTables.Get(tableName)
		if !ok {
			sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
				"Requested resource not found: Table: %s not found", tableName)
			return
		}
		for _, op := range ops {
			switch {
			case op.PutRequest != nil:
				key := ddbItemKey(t, op.PutRequest.Item)
				ddbItems.Put(key, op.PutRequest.Item)
				ddbItemNames.Put(key, key)
			case op.DeleteRequest != nil:
				key := ddbItemKey(t, op.DeleteRequest.Key)
				ddbItems.Delete(key)
				ddbItemNames.Delete(key)
			}
		}
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{"UnprocessedItems": map[string]any{}})
}

func handleDDBBatchGetItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RequestItems map[string]struct {
			Keys []map[string]any `json:"Keys"`
		} `json:"RequestItems"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()
	responses := map[string][]map[string]any{}
	for tableName, spec := range req.RequestItems {
		t, ok := ddbTables.Get(tableName)
		if !ok {
			sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
				"Requested resource not found: Table: %s not found", tableName)
			return
		}
		items := []map[string]any{}
		for _, key := range spec.Keys {
			if it, ok := ddbItems.Get(ddbItemKey(t, key)); ok {
				items = append(items, it)
			}
		}
		responses[tableName] = items
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{
		"Responses":       responses,
		"UnprocessedKeys": map[string]any{},
	})
}

// handleDDBTransactWriteItems applies Put/Delete/Update/ConditionCheck actions
// atomically: all ConditionExpressions are evaluated first under the item lock;
// if any fails the whole transaction aborts with TransactionCanceledException.
func handleDDBTransactWriteItems(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TransactItems []struct {
			Put *struct {
				TableName                 string            `json:"TableName"`
				Item                      map[string]any    `json:"Item"`
				ConditionExpression       string            `json:"ConditionExpression"`
				ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames"`
				ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues"`
			} `json:"Put"`
			Delete *struct {
				TableName                 string            `json:"TableName"`
				Key                       map[string]any    `json:"Key"`
				ConditionExpression       string            `json:"ConditionExpression"`
				ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames"`
				ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues"`
			} `json:"Delete"`
			ConditionCheck *struct {
				TableName                 string            `json:"TableName"`
				Key                       map[string]any    `json:"Key"`
				ConditionExpression       string            `json:"ConditionExpression"`
				ExpressionAttributeNames  map[string]string `json:"ExpressionAttributeNames"`
				ExpressionAttributeValues map[string]any    `json:"ExpressionAttributeValues"`
			} `json:"ConditionCheck"`
		} `json:"TransactItems"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()

	// Validate every condition before mutating anything.
	for _, ti := range req.TransactItems {
		var tableName, condExpr, storeKey string
		var keyItem map[string]any
		var names map[string]string
		var values map[string]any
		switch {
		case ti.Put != nil:
			tableName, condExpr, keyItem, names, values = ti.Put.TableName, ti.Put.ConditionExpression, ti.Put.Item, ti.Put.ExpressionAttributeNames, ti.Put.ExpressionAttributeValues
		case ti.Delete != nil:
			tableName, condExpr, keyItem, names, values = ti.Delete.TableName, ti.Delete.ConditionExpression, ti.Delete.Key, ti.Delete.ExpressionAttributeNames, ti.Delete.ExpressionAttributeValues
		case ti.ConditionCheck != nil:
			tableName, condExpr, keyItem, names, values = ti.ConditionCheck.TableName, ti.ConditionCheck.ConditionExpression, ti.ConditionCheck.Key, ti.ConditionCheck.ExpressionAttributeNames, ti.ConditionCheck.ExpressionAttributeValues
		default:
			continue
		}
		t, ok := ddbTables.Get(tableName)
		if !ok {
			sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
				"Requested resource not found: Table: %s not found", tableName)
			return
		}
		storeKey = ddbItemKey(t, keyItem)
		current, exists := ddbItems.Get(storeKey)
		if condExpr != "" && !ddbEvalCondition(current, exists, condExpr, names, values) {
			sim.AWSError(w, "TransactionCanceledException",
				"Transaction cancelled, please refer cancellation reasons for specific reasons [ConditionalCheckFailed]",
				http.StatusBadRequest)
			return
		}
	}

	// Apply mutations.
	for _, ti := range req.TransactItems {
		switch {
		case ti.Put != nil:
			t, _ := ddbTables.Get(ti.Put.TableName)
			key := ddbItemKey(t, ti.Put.Item)
			ddbItems.Put(key, ti.Put.Item)
			ddbItemNames.Put(key, key)
		case ti.Delete != nil:
			t, _ := ddbTables.Get(ti.Delete.TableName)
			key := ddbItemKey(t, ti.Delete.Key)
			ddbItems.Delete(key)
			ddbItemNames.Delete(key)
		}
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{})
}

func handleDDBTransactGetItems(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TransactItems []struct {
			Get *struct {
				TableName string         `json:"TableName"`
				Key       map[string]any `json:"Key"`
			} `json:"Get"`
		} `json:"TransactItems"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	ddbItemsMu.Lock()
	defer ddbItemsMu.Unlock()
	responses := make([]map[string]any, 0, len(req.TransactItems))
	for _, ti := range req.TransactItems {
		if ti.Get == nil {
			responses = append(responses, map[string]any{})
			continue
		}
		t, ok := ddbTables.Get(ti.Get.TableName)
		if !ok {
			sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
				"Requested resource not found: Table: %s not found", ti.Get.TableName)
			return
		}
		if it, ok := ddbItems.Get(ddbItemKey(t, ti.Get.Key)); ok {
			responses = append(responses, map[string]any{"Item": it})
		} else {
			responses = append(responses, map[string]any{})
		}
	}
	writeDDBJSON(w, http.StatusOK, map[string]any{"Responses": responses})
}

// ddbExtractKey builds a DynamoDB key AttributeValue map from an item's primary key attributes.
func ddbExtractKey(t DDBTable, item map[string]any) map[string]any {
	key := map[string]any{}
	for _, k := range t.KeySchema {
		if v, ok := item[k.AttributeName]; ok {
			key[k.AttributeName] = v
		}
	}
	return key
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

func ddbMatchesExpression(table DDBTable, item map[string]any, expr string, names map[string]string, values map[string]any) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	parts := strings.Split(expr, " AND ")
	if len(parts) == 1 {
		parts = strings.Split(expr, " and ")
	}
	for _, part := range parts {
		left, right, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return false
		}
		attr := ddbResolveAttrName(strings.TrimSpace(left), names)
		token := strings.TrimSpace(right)
		want, ok := values[token]
		if !ok {
			return false
		}
		if !ddbAttrValuesEqual(item[attr], want) {
			return false
		}
	}
	_ = table
	return true
}

func ddbResolveAttrName(name string, aliases map[string]string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "#") {
		if v := aliases[name]; v != "" {
			return v
		}
	}
	return name
}

func ddbAttrValuesEqual(a, b any) bool {
	av, aok := a.(map[string]any)
	bv, bok := b.(map[string]any)
	if !aok || !bok {
		return fmt.Sprint(a) == fmt.Sprint(b)
	}
	for _, key := range []string{"S", "N", "B", "BOOL", "NULL"} {
		if fmt.Sprint(av[key]) != fmt.Sprint(bv[key]) {
			return false
		}
		if _, ok := av[key]; ok {
			return true
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// ddbItemNames mirrors the keys of ddbItems for iteration. Maintained
// alongside Put/Delete in handleDDBPutItem etc.
var ddbItemNames sim.Store[string]
