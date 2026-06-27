package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	structpb "google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	// Pure-Go SQLite driver — same one sim.Store relies on. Registering it
	// here keeps the data plane self-contained even though the parent binary
	// already pulls the driver in transitively.
	_ "modernc.org/sqlite"
)

// Cloud Spanner gRPC data plane (google.spanner.v1.Spanner). The admin REST
// slice in spanner.go owns the instance/database/DDL/session stores; every
// RPC here reads those same stores so the REST and gRPC surfaces observe one
// consistent cloud state. The high-level cloud.google.com/go/spanner client is
// gRPC-only and reaches the simulator through SPANNER_EMULATOR_HOST, the same
// coordinate it uses for Google's own Spanner emulator.
//
// ExecuteSql / ExecuteStreamingSql / Read / StreamingRead / Commit execute
// against a real in-memory SQLite engine that is materialized from each
// database's CREATE TABLE DDL. A CREATE TABLE followed by an INSERT followed by
// a SELECT therefore returns the inserted row — no synthetic result sets. SQL
// constructs Spanner supports that SQLite cannot express (interleaved tables,
// proto-typed columns, generated columns) are rejected with a loud error at DDL
// time so the untranslatable table is simply absent from the backing store,
// rather than returning empty rows at query time.

type spannerDataGRPC struct {
	sppb.UnimplementedSpannerServer
}

func registerSpannerGRPC(gs *grpc.Server) {
	sppb.RegisterSpannerServer(gs, &spannerDataGRPC{})
}

// ---------------------------------------------------------------------------
// per-database SQLite backing engine
// ---------------------------------------------------------------------------

// spannerBackend holds the materialized SQLite engine for one database plus
// the count of DDL statements already applied, so a subsequent
// UpdateDatabaseDdl that appends statements can be reconciled incrementally.
type spannerBackend struct {
	mu              sync.Mutex
	db              *sql.DB
	appliedDDLCount int
}

var (
	spannerBackends      = map[string]*spannerBackend{}
	spannerBackendsMutex sync.Mutex
	// spannerTxns tracks begun read-write transaction ids and the database they
	// belong to. A process restart drops all in-flight transactions, which is
	// faithful to a real Spanner session pool losing its session on restart; it
	// does not need the on-disk persistence the resource stores use.
	spannerTxns = sim.NewStateStore[spannerTxnState]()
)

// spannerTxnState records the database a begun transaction is pinned to so
// Commit / Rollback can reject a transaction used against a different session.
type spannerTxnState struct {
	Database string `json:"database"`
	ReadOnly bool   `json:"readOnly"`
}

// spannerBackendFor returns the materialized SQLite backend for a database,
// creating it and applying any pending DDL on first access. DDL statements
// beyond what has already been applied are reconciled incrementally, mirroring
// how UpdateDatabaseDdl extends a database's schema over time.
func spannerBackendFor(dbName string) (*spannerBackend, error) {
	spannerBackendsMutex.Lock()
	defer spannerBackendsMutex.Unlock()
	b, ok := spannerBackends[dbName]
	if !ok {
		// In-memory DB. The unique shared cache name keeps each database
		// isolated; modernc honors file::memory:?cache=shared semantics with a
		// unique id so concurrent databases do not collide.
		sqlDB, err := sql.Open("sqlite", "file:"+dbName+"?mode=memory&cache=shared")
		if err != nil {
			return nil, status.Errorf(codes.Internal, "open backing store for %s: %v", dbName, err)
		}
		if err := sqlDB.Ping(); err != nil {
			_ = sqlDB.Close()
			return nil, status.Errorf(codes.Internal, "ping backing store for %s: %v", dbName, err)
		}
		b = &spannerBackend{db: sqlDB}
		spannerBackends[dbName] = b
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ddl, hasDDL := spannerDDLs.Get(dbName)
	if !hasDDL || len(ddl.Statements) == 0 {
		return b, nil
	}
	for i := b.appliedDDLCount; i < len(ddl.Statements); i++ {
		stmt := ddl.Statements[i]
		translated, ok := spannerTranslateDDL(stmt)
		if !ok {
			// Honest degradation: an untranslatable statement is skipped with a
			// logged warning, so the table it would have created is simply
			// absent and queries against it fail loudly at execute time.
			fmt.Fprintf(simStderr(), "spanner: skipping untranslatable DDL for %s: %s\n", dbName, stmt)
			continue
		}
		if _, err := b.db.Exec(translated); err != nil {
			fmt.Fprintf(simStderr(), "spanner: DDL apply failed for %s (%q): %v\n", dbName, translated, err)
		}
	}
	b.appliedDDLCount = len(ddl.Statements)
	return b, nil
}

// simStderr returns os.Stderr; wrapped so tests can stub it if needed.
func simStderr() *os.File {
	return os.Stderr
}

// ---------------------------------------------------------------------------
// DDL translation: Spanner SQL dialect → SQLite
// ---------------------------------------------------------------------------

var (
	spannerInterleaveRe = regexp.MustCompile(`(?is)\s*,?\s*INTERLEAVE\s+IN\s+PARENT\b.*$`)
)

// spannerTranslateDDL converts a Spanner DDL statement to a SQLite-equivalent
// one. It currently handles CREATE TABLE and DROP TABLE / CREATE / DROP INDEX
// faithfully; other statements (ALTER TABLE, CREATE VIEW, inter-leaved DDL)
// return ok=false so the caller can skip them loudly rather than fake support.
func spannerTranslateDDL(stmt string) (string, bool) {
	trimmed := strings.TrimSpace(stmt)
	upper := strings.ToUpper(trimmed)
	switch {
	case strings.HasPrefix(upper, "CREATE TABLE"):
		return spannerTranslateCreateTable(trimmed)
	case strings.HasPrefix(upper, "DROP TABLE"):
		return spannerRewriteTypes(trimmed), true
	case strings.HasPrefix(upper, "CREATE") && strings.Contains(upper, "INDEX"):
		return trimmed, true
	case strings.HasPrefix(upper, "DROP INDEX"):
		return trimmed, true
	}
	return "", false
}

// spannerCreateTableHeadRe captures the CREATE TABLE keyword, the table name,
// and everything after the opening paren of the column list. The column list's
// matching close paren is located by balanced paren scanning in the caller, so
// nested parens inside column types (STRING(100), ARRAY<...>) survive.
var spannerCreateTableHeadRe = regexp.MustCompile(`(?is)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`?" + `([A-Za-z0-9_\.]+)` + "`?" + `\s*\((.*)$`)

// spannerTranslateCreateTable rewrites a Spanner CREATE TABLE into a SQLite one
// the engine will accept: Spanner-specific column types are mapped onto
// SQLite's dynamic typing, and clauses SQLite has no analogue for (INTERLEAVE
// IN PARENT, ON DELETE CASCADE) are dropped. The trailing PRIMARY KEY (...)
// clause Spanner places after the column list is moved inside the parentheses
// as a table-level constraint, which is where SQLite expects it.
func spannerTranslateCreateTable(stmt string) (string, bool) {
	m := spannerCreateTableHeadRe.FindStringSubmatch(stmt)
	if m == nil {
		return "", false
	}
	table := m[1]
	rest := m[2]
	// Scan for the matching close paren of the column list, accounting for
	// nested parens (STRING(100), ARRAY<...> when rewritten, etc.).
	depth := 1
	close := -1
	for i, r := range rest {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
			}
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 {
		return "", false
	}
	body := rest[:close]
	trailing := strings.TrimSpace(rest[close+1:])
	trailing = spannerInterleaveRe.ReplaceAllString(trailing, "")
	trailing = regexp.MustCompile(`(?is)\s*,?\s*ON\s+DELETE\s+\w+\s*\(?[^)]*\)?`).ReplaceAllString(trailing, "")
	trailing = strings.TrimSpace(strings.Trim(strings.TrimSpace(trailing), ","))
	body = spannerRewriteTypes(body)
	inner := body
	if trailing != "" {
		inner += ", " + trailing
	}
	return "CREATE TABLE " + quoteIdent(table) + " (" + inner + ")", true
}

// spannerTypeRe matches Spanner column type tokens. It also handles the
// column-name prefix so the rewrite leaves names intact.
var spannerTypeRe = regexp.MustCompile(`(?i)\b(ARRAY\s*<[^>]+>|STRING\s*\(\s*(?:MAX|\d+)\s*\)|BYTES\s*\(\s*(?:MAX|\d+)\s*\)|INT64|FLOAT64|BOOL|TIMESTAMP|DATE|NUMERIC|JSON|TOKENLIST)\b`)

func spannerRewriteTypes(s string) string {
	return spannerTypeRe.ReplaceAllStringFunc(s, func(tok string) string {
		upper := strings.ToUpper(strings.TrimSpace(tok))
		switch {
		case strings.HasPrefix(upper, "INT64"):
			return "INTEGER"
		case strings.HasPrefix(upper, "FLOAT64"), strings.HasPrefix(upper, "NUMERIC"):
			return "REAL"
		case strings.HasPrefix(upper, "BOOL"):
			return "INTEGER"
		case strings.HasPrefix(upper, "BYTES"):
			return "BLOB"
		case strings.HasPrefix(upper, "ARRAY"):
			// Arrays are stored as a JSON-encoded TEXT column.
			return "TEXT"
		case strings.HasPrefix(upper, "STRING"),
			strings.HasPrefix(upper, "TIMESTAMP"),
			strings.HasPrefix(upper, "DATE"),
			strings.HasPrefix(upper, "JSON"),
			strings.HasPrefix(upper, "TOKENLIST"):
			return "TEXT"
		}
		return tok
	})
}

func quoteIdent(name string) string {
	return "\"" + strings.ReplaceAll(name, "\"", "\"\"") + "\""
}

// ---------------------------------------------------------------------------
// path / name normalization
// ---------------------------------------------------------------------------

// spannerSessionDatabase resolves a session name to its parent database full
// name (projects/.../databases/...). Returns "" if the session does not exist.
func spannerSessionDatabase(sessionName string) (string, error) {
	sess, ok := spannerSessions.Get(sessionName)
	if !ok {
		return "", status.Errorf(codes.NotFound, "session not found: %s", sessionName)
	}
	// Session name = projects/{p}/instances/{i}/databases/{d}/sessions/{s}
	idx := strings.LastIndex(sess.Name, "/sessions/")
	if idx < 0 {
		return "", status.Errorf(codes.InvalidArgument, "malformed session name: %s", sess.Name)
	}
	return sess.Name[:idx], nil
}

// ---------------------------------------------------------------------------
// proto Value <-> Go value conversion (per Spanner Type)
// ---------------------------------------------------------------------------

// spannerProtoToGo converts a proto Value to a Go value suitable for binding
// to a SQLite parameter, guided by the column's Spanner type. INT64 values may
// arrive as string_value (canonical Spanner encoding) or number_value; both are
// honored.
func spannerProtoToGo(v *structpb.Value, t *sppb.Type) any {
	if v == nil {
		return nil
	}
	switch t.GetCode() {
	case sppb.TypeCode_INT64:
		switch k := v.Kind.(type) {
		case *structpb.Value_StringValue:
			if n, err := strconv.ParseInt(k.StringValue, 10, 64); err == nil {
				return n
			}
			return k.StringValue
		case *structpb.Value_NumberValue:
			return int64(k.NumberValue)
		case *structpb.Value_NullValue:
			return nil
		}
	case sppb.TypeCode_FLOAT64, sppb.TypeCode_NUMERIC:
		switch k := v.Kind.(type) {
		case *structpb.Value_NumberValue:
			return k.NumberValue
		case *structpb.Value_StringValue:
			if f, err := strconv.ParseFloat(k.StringValue, 64); err == nil {
				return f
			}
			return k.StringValue
		case *structpb.Value_NullValue:
			return nil
		}
	case sppb.TypeCode_BOOL:
		switch k := v.Kind.(type) {
		case *structpb.Value_BoolValue:
			return k.BoolValue
		case *structpb.Value_NullValue:
			return nil
		}
	case sppb.TypeCode_BYTES:
		switch k := v.Kind.(type) {
		case *structpb.Value_StringValue:
			if b, err := base64.StdEncoding.DecodeString(k.StringValue); err == nil {
				return b
			}
			return []byte(k.StringValue)
		case *structpb.Value_NullValue:
			return nil
		}
	case sppb.TypeCode_ARRAY:
		// Arrays bind as a JSON string in the TEXT column.
		switch k := v.Kind.(type) {
		case *structpb.Value_ListValue:
			return spannerArrayToJSON(k.ListValue)
		case *structpb.Value_NullValue:
			return nil
		}
	}
	// STRING, DATE, TIMESTAMP, JSON and anything else: take the string form.
	switch k := v.Kind.(type) {
	case *structpb.Value_StringValue:
		return k.StringValue
	case *structpb.Value_NullValue:
		return nil
	case *structpb.Value_NumberValue:
		// No type hint — let SQLite store the number natively.
		return k.NumberValue
	case *structpb.Value_BoolValue:
		return k.BoolValue
	}
	return nil
}

func spannerArrayToJSON(lv *structpb.ListValue) string {
	parts := make([]string, 0, len(lv.GetValues()))
	for _, e := range lv.GetValues() {
		switch k := e.Kind.(type) {
		case *structpb.Value_StringValue:
			b, _ := json.Marshal(k.StringValue)
			parts = append(parts, string(b))
		case *structpb.Value_NumberValue:
			parts = append(parts, strconv.FormatFloat(k.NumberValue, 'f', -1, 64))
		case *structpb.Value_BoolValue:
			parts = append(parts, strconv.FormatBool(k.BoolValue))
		case *structpb.Value_NullValue:
			parts = append(parts, "null")
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// spannerGoToProto converts a Go value read from SQLite back to a proto Value,
// guided by the column's Spanner type.
func spannerGoToProto(v any, t *sppb.Type) *structpb.Value {
	if v == nil {
		return &structpb.Value{Kind: &structpb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}}
	}
	switch t.GetCode() {
	case sppb.TypeCode_INT64:
		switch n := v.(type) {
		case int64:
			return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(n, 10)}}
		case int:
			return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(int64(n), 10)}}
		case float64:
			return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(int64(n), 10)}}
		}
	case sppb.TypeCode_FLOAT64, sppb.TypeCode_NUMERIC:
		if f, ok := v.(float64); ok {
			return &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: f}}
		}
	case sppb.TypeCode_BOOL:
		switch b := v.(type) {
		case bool:
			return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: b}}
		case int64:
			return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: b != 0}}
		case float64:
			return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: int64(b) != 0}}
		}
	case sppb.TypeCode_BYTES:
		if b, ok := v.([]byte); ok {
			return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: base64.StdEncoding.EncodeToString(b)}}
		}
	case sppb.TypeCode_ARRAY:
		if s, ok := v.(string); ok {
			return spannerArrayFromString(s, t.GetArrayElementType())
		}
	}
	// STRING / DATE / TIMESTAMP / JSON / fallback: text representation.
	return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}}
}

// spannerArrayFromString parses a JSON array stored in a TEXT column back into a
// proto ListValue of the element type.
func spannerArrayFromString(s string, elem *sppb.Type) *structpb.Value {
	var raw []any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return &structpb.Value{Kind: &structpb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}}
	}
	out := &structpb.ListValue{Values: make([]*structpb.Value, 0, len(raw))}
	for _, e := range raw {
		out.Values = append(out.Values, anyValueToProto(e, elem))
	}
	return &structpb.Value{Kind: &structpb.Value_ListValue{ListValue: out}}
}

func anyValueToProto(e any, t *sppb.Type) *structpb.Value {
	switch x := e.(type) {
	case string:
		return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: x}}
	case float64:
		if t != nil && t.GetCode() == sppb.TypeCode_INT64 {
			return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: strconv.FormatInt(int64(x), 10)}}
		}
		return &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: x}}
	case bool:
		return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: x}}
	case nil:
		return &structpb.Value{Kind: &structpb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}}
	}
	return &structpb.Value{Kind: &structpb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}}
}

// spannerTypeForSQLite maps a SQLite declared column type to the closest
// Spanner Type.
func spannerTypeForSQLite(declType string) *sppb.Type {
	upper := strings.ToUpper(strings.TrimSpace(declType))
	switch {
	case strings.Contains(upper, "INT"):
		return &sppb.Type{Code: sppb.TypeCode_INT64}
	case strings.Contains(upper, "REAL") || strings.Contains(upper, "FLOA") || strings.Contains(upper, "DOUB"):
		return &sppb.Type{Code: sppb.TypeCode_FLOAT64}
	case strings.Contains(upper, "BLOB"):
		return &sppb.Type{Code: sppb.TypeCode_BYTES}
	case strings.Contains(upper, "BOOL"):
		return &sppb.Type{Code: sppb.TypeCode_BOOL}
	}
	return &sppb.Type{Code: sppb.TypeCode_STRING}
}

// ---------------------------------------------------------------------------
// metadata helpers
// ---------------------------------------------------------------------------

// spannerTableColumns returns the SQLite column names and Spanner types for a
// table, in declared order. Used to shape ResultSet row types and to coerce
// mutation values to the right Go type.
func spannerTableColumns(b *spannerBackend, table string) ([]string, []*sppb.Type, error) {
	rows, err := b.db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "read table_info for %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	var types []*sppb.Type
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, nil, status.Errorf(codes.Internal, "scan table_info: %v", err)
		}
		names = append(names, name)
		types = append(types, spannerTypeForSQLite(ctype))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, status.Errorf(codes.Internal, "table_info rows: %v", err)
	}
	if len(names) == 0 {
		return nil, nil, status.Errorf(codes.NotFound, "table not found: %s", table)
	}
	return names, types, nil
}

// spannerPrimaryKeyColumns returns the primary-key column names of a table in
// SQLite-defined order. Used to scope UPDATE/DELETE mutations and point reads.
func spannerPrimaryKeyColumns(b *spannerBackend, table string) ([]string, error) {
	rows, err := b.db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read table_info for %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var pkCols []string
	maxPK := 0
	pkOrder := map[string]int{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, status.Errorf(codes.Internal, "scan table_info: %v", err)
		}
		if pk > 0 {
			pkOrder[name] = pk
			if pk > maxPK {
				maxPK = pk
			}
		}
	}
	// Emit in pk-index order.
	for i := 1; i <= maxPK; i++ {
		for name, idx := range pkOrder {
			if idx == i {
				pkCols = append(pkCols, name)
			}
		}
	}
	if len(pkCols) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "table %s has no primary key; mutations and point reads require one", table)
	}
	return pkCols, nil
}

// ---------------------------------------------------------------------------
// query execution core
// ---------------------------------------------------------------------------

// spannerExecResult captures everything needed to shape a ResultSet: the column
// names, their Spanner types, and the materialized row values (already in proto
// form).
type spannerExecResult struct {
	columns []string
	types   []*sppb.Type
	rows    []*structpb.ListValue
}

// spannerRunQuery executes a SQL statement (with optional bind args) against
// the database's SQLite backend and returns its rows shaped for Spanner's
// ResultSet. The column types are derived from the SQLite result columns; for
// SELECTs these come from the declared schema, for expressions they default to
// STRING. Args may be sql.NamedArg (for @name placeholders in ExecuteSql) or
// plain positional values (for ? placeholders in Read).
func spannerRunQuery(ctx context.Context, b *spannerBackend, sqlText string, args []any) (*spannerExecResult, error) {
	q, err := b.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "execute sql: %v", err)
	}
	defer func() { _ = q.Close() }()

	colNames, err := q.Columns()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "result columns: %v", err)
	}
	colTypes, err := q.ColumnTypes()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "column types: %v", err)
	}
	res := &spannerExecResult{
		columns: colNames,
		types:   make([]*sppb.Type, len(colNames)),
	}
	// Prefer the table's declared types (richer than the runtime affinity SQLite
	// reports via ColumnTypes.DatabaseTypeName, which is often blank for
	// expressions). For a plain SELECT col1, col2 FROM table the declared type
	// is recoverable by mapping the column name back through the table schema;
	// for expressions we fall back to DatabaseTypeName.
	tableColTypes := spannerLookupSelectTypes(b, sqlText, colNames, colTypes)
	for i := range colNames {
		if tableColTypes != nil && tableColTypes[i] != nil {
			res.types[i] = tableColTypes[i]
		} else {
			res.types[i] = spannerTypeForSQLite(spannerColumnDeclType(colTypes[i]))
		}
	}

	for q.Next() {
		raw := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := q.Scan(ptrs...); err != nil {
			return nil, status.Errorf(codes.Internal, "scan row: %v", err)
		}
		lv := &structpb.ListValue{Values: make([]*structpb.Value, len(raw))}
		for i, v := range raw {
			lv.Values[i] = spannerGoToProto(spannerNormalizeScanValue(v), res.types[i])
		}
		res.rows = append(res.rows, lv)
	}
	if err := q.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "query rows: %v", err)
	}
	return res, nil
}

// spannerColumnDeclType extracts the declared type name a *sql.ColumnType
// reports, falling back to an empty string.
func spannerColumnDeclType(ct *sql.ColumnType) string {
	if ct == nil {
		return ""
	}
	return ct.DatabaseTypeName()
}

// spannerBindArgs turns the params Struct into a slice of sql.NamedArg bound to
// the @name placeholders Spanner SQL uses. SQLite (modernc) honors @name named
// parameters natively, so the SQL text is passed through unchanged.
func spannerBindArgs(params *structpb.Struct, paramTypes map[string]*sppb.Type) []any {
	if params == nil {
		return nil
	}
	args := make([]any, 0, len(params.GetFields()))
	for name, val := range params.GetFields() {
		t := paramTypes[name]
		args = append(args, sql.Named(name, spannerProtoToGo(val, t)))
	}
	return args
}

// spannerNormalizeScanValue coerces a value read out of SQLite into one of the
// canonical Go types the proto converter understands.
func spannerNormalizeScanValue(v any) any {
	switch x := v.(type) {
	case []byte:
		// SQLite returns TEXT as []byte; turn it back into a string so it shapes
		// as a Spanner STRING rather than BYTES.
		return string(x)
	}
	return v
}

// spannerLookupSelectTypes best-effort maps a SELECT's result columns to the
// declared Spanner types of the underlying table columns, by recognizing the
// common "SELECT cols FROM table" shape the high-level client emits for point
// and range reads. Returns nil if the shape is not recognized.
var spannerSimpleSelectRe = regexp.MustCompile(`(?is)^\s*SELECT\s+(.+?)\s+FROM\s+([A-Za-z0-9_\.]+)(?:\s|$)`)

func spannerLookupSelectTypes(b *spannerBackend, sqlText string, colNames []string, colTypes []*sql.ColumnType) []*sppb.Type {
	m := spannerSimpleSelectRe.FindStringSubmatch(sqlText)
	if m == nil {
		return nil
	}
	colsPart := strings.TrimSpace(m[1])
	table := strings.Trim(m[2], "`\"")
	// Only handle the simple comma-separated column list (no expressions, no
	// aliases); anything else falls through to the runtime type guess.
	var want []string
	for _, c := range strings.Split(colsPart, ",") {
		want = append(want, strings.Trim(strings.TrimSpace(c), "`\""))
	}
	declNames, declTypes, err := spannerTableColumns(b, table)
	if err != nil {
		return nil
	}
	byName := map[string]*sppb.Type{}
	for i, n := range declNames {
		byName[n] = declTypes[i]
	}
	out := make([]*sppb.Type, len(colNames))
	for i, name := range colNames {
		// Map by position when the request column order matches the SELECT list
		// order; otherwise resolve by name.
		if i < len(want) {
			if t, ok := byName[want[i]]; ok {
				out[i] = t
				continue
			}
		}
		if t, ok := byName[name]; ok {
			out[i] = t
		}
	}
	return out
}

// spannerShapeResultSet builds a non-streaming ResultSet from an exec result.
func spannerShapeResultSet(r *spannerExecResult) *sppb.ResultSet {
	fields := make([]*sppb.StructType_Field, len(r.columns))
	for i, name := range r.columns {
		fields[i] = &sppb.StructType_Field{Name: name, Type: r.types[i]}
	}
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}},
		Rows:     r.rows,
	}
}

// ---------------------------------------------------------------------------
// Sessions RPCs
// ---------------------------------------------------------------------------

// spannerResolveTxn interprets a request's TransactionSelector. When the
// selector asks to begin a transaction, a new id is minted and recorded against
// the database, and begun=true so the caller can surface it in the response
// metadata (the high-level client relies on this inline-begin path for
// ReadWriteTransaction). A selector carrying an existing id is validated against
// the store; single-use and absent selectors are no-ops.
func spannerResolveTxn(dbName string, sel *sppb.TransactionSelector) (id []byte, begun bool, err error) {
	if sel == nil {
		return nil, false, nil
	}
	switch sel.Selector.(type) {
	case *sppb.TransactionSelector_Begin:
		newID := generateUUID()
		spannerTxns.Put(newID, spannerTxnState{Database: dbName})
		return []byte(newID), true, nil
	case *sppb.TransactionSelector_Id:
		st, ok := spannerTxns.Get(string(sel.GetId()))
		if !ok {
			return nil, false, status.Error(codes.InvalidArgument, "invalid transaction id")
		}
		if st.Database != dbName {
			return nil, false, status.Error(codes.InvalidArgument, "transaction does not belong to this session's database")
		}
		return sel.GetId(), false, nil
	}
	return nil, false, nil
}

func (s *spannerDataGRPC) CreateSession(_ context.Context, req *sppb.CreateSessionRequest) (*sppb.Session, error) {
	dbName := req.GetDatabase()
	if _, ok := spannerDatabases.Get(dbName); !ok {
		return nil, status.Errorf(codes.NotFound, "database not found: %s", dbName)
	}
	if _, err := spannerBackendFor(dbName); err != nil {
		return nil, err
	}
	sess := req.GetSession()
	if sess == nil {
		sess = &sppb.Session{}
	}
	sessionID := generateUUID()
	out := &sppb.Session{
		Name:        dbName + "/sessions/" + sessionID,
		Labels:      sess.GetLabels(),
		CreateTime:  timestamppb.Now(),
		CreatorRole: sess.GetCreatorRole(),
	}
	spannerSessions.Put(out.Name, spannerSession{
		Name:       out.Name,
		CreateTime: time.Now().UTC().Format(time.RFC3339Nano),
		Labels:     out.Labels,
	})
	return out, nil
}

func (s *spannerDataGRPC) BatchCreateSessions(_ context.Context, req *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
	dbName := req.GetDatabase()
	if _, ok := spannerDatabases.Get(dbName); !ok {
		return nil, status.Errorf(codes.NotFound, "database not found: %s", dbName)
	}
	if _, err := spannerBackendFor(dbName); err != nil {
		return nil, err
	}
	count := int(req.GetSessionCount())
	if count <= 0 {
		count = 1
	}
	tmpl := req.GetSessionTemplate()
	resp := &sppb.BatchCreateSessionsResponse{Session: make([]*sppb.Session, 0, count)}
	for i := 0; i < count; i++ {
		sessionID := generateUUID()
		sess := &sppb.Session{
			Name:        dbName + "/sessions/" + sessionID,
			Labels:      tmpl.GetLabels(),
			CreateTime:  timestamppb.Now(),
			CreatorRole: tmpl.GetCreatorRole(),
		}
		spannerSessions.Put(sess.Name, spannerSession{
			Name:       sess.Name,
			CreateTime: time.Now().UTC().Format(time.RFC3339Nano),
			Labels:     sess.Labels,
		})
		resp.Session = append(resp.Session, sess)
	}
	return resp, nil
}

func (s *spannerDataGRPC) GetSession(_ context.Context, req *sppb.GetSessionRequest) (*sppb.Session, error) {
	stored, ok := spannerSessions.Get(req.GetName())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "session not found: %s", req.GetName())
	}
	return spannerStoredSessionToProto(stored), nil
}

func (s *spannerDataGRPC) ListSessions(_ context.Context, req *sppb.ListSessionsRequest) (*sppb.ListSessionsResponse, error) {
	prefix := req.GetDatabase() + "/sessions/"
	out := spannerSessions.Filter(func(sess spannerSession) bool { return strings.HasPrefix(sess.Name, prefix) })
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	resp := &sppb.ListSessionsResponse{Sessions: make([]*sppb.Session, 0, len(out))}
	for _, sess := range out {
		resp.Sessions = append(resp.Sessions, spannerStoredSessionToProto(sess))
	}
	return resp, nil
}

func (s *spannerDataGRPC) DeleteSession(_ context.Context, req *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	if !spannerSessions.Delete(req.GetName()) {
		return nil, status.Errorf(codes.NotFound, "session not found: %s", req.GetName())
	}
	return &emptypb.Empty{}, nil
}

func spannerStoredSessionToProto(s spannerSession) *sppb.Session {
	out := &sppb.Session{Name: s.Name, Labels: s.Labels}
	if s.CreateTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, s.CreateTime); err == nil {
			out.CreateTime = timestamppb.New(t.UTC())
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// ExecuteSql / ExecuteStreamingSql
// ---------------------------------------------------------------------------

func (s *spannerDataGRPC) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return nil, err
	}
	b, err := spannerBackendFor(dbName)
	if err != nil {
		return nil, err
	}
	res, err := spannerRunQuery(ctx, b, req.GetSql(), spannerBindArgs(req.GetParams(), req.GetParamTypes()))
	if err != nil {
		return nil, err
	}
	rs := spannerShapeResultSet(res)
	if txnID, begun, err := spannerResolveTxn(dbName, req.GetTransaction()); err != nil {
		return nil, err
	} else if begun {
		rs.Metadata.Transaction = &sppb.Transaction{Id: txnID}
	}
	return rs, nil
}

func (s *spannerDataGRPC) ExecuteStreamingSql(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	ctx := stream.Context()
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return err
	}
	b, err := spannerBackendFor(dbName)
	if err != nil {
		return err
	}
	res, err := spannerRunQuery(ctx, b, req.GetSql(), spannerBindArgs(req.GetParams(), req.GetParamTypes()))
	if err != nil {
		return err
	}
	txnID, begun, err := spannerResolveTxn(dbName, req.GetTransaction())
	if err != nil {
		return err
	}
	fields := make([]*sppb.StructType_Field, len(res.columns))
	for i, name := range res.columns {
		fields[i] = &sppb.StructType_Field{Name: name, Type: res.types[i]}
	}
	// First chunk carries the metadata so the client can shape the stream.
	first := &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}},
	}
	if begun {
		first.Metadata.Transaction = &sppb.Transaction{Id: txnID}
	}
	if err := stream.Send(first); err != nil {
		return err
	}
	// Stream rows as flattened values (Spanner's PartialResultSet merges
	// per-row values into one flat list). One row per chunk keeps the shaping
	// simple and faithful.
	for _, row := range res.rows {
		chunk := &sppb.PartialResultSet{Values: row.GetValues()}
		if err := stream.Send(chunk); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Read / StreamingRead
// ---------------------------------------------------------------------------

// spannerBuildReadQuery translates a Spanner Read request into a parameterized
// SQLite SELECT plus the bind args. Only the shapes the high-level client emits
// are translated: AllKeys, a flat point-key list, and (single-column) ranges.
// Multi-column range reads return a loud Unimplemented rather than a wrong
// result.
func spannerBuildReadQuery(b *spannerBackend, req *sppb.ReadRequest) (string, []any, error) {
	table := req.GetTable()
	cols := req.GetColumns()
	if table == "" {
		return "", nil, status.Error(codes.InvalidArgument, "table is required")
	}
	pkCols, err := spannerPrimaryKeyColumns(b, table)
	if err != nil {
		return "", nil, err
	}
	colList := "*"
	if len(cols) > 0 {
		quoted := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = quoteIdent(c)
		}
		colList = strings.Join(quoted, ", ")
	}
	var args []any
	var where string
	ks := req.GetKeySet()
	if ks == nil || ks.GetAll() {
		where = ""
	} else if len(ks.GetKeys()) > 0 {
		// Point keys. Each key is a ListValue of the PK columns in declared
		// order. Build (pk1=? AND pk2=?) OR (pk1=? AND pk2=?) …
		declNames, declTypes, err := spannerTableColumns(b, table)
		if err != nil {
			return "", nil, err
		}
		pkIndex := map[int]int{} // pk position -> columns-table index
		for i, n := range declNames {
			for pi, pk := range pkCols {
				if n == pk {
					pkIndex[pi] = i
				}
			}
		}
		var ors []string
		for _, key := range ks.GetKeys() {
			vals := key.GetValues()
			if len(vals) < len(pkCols) {
				return "", nil, status.Errorf(codes.InvalidArgument, "key has %d values, expected %d", len(vals), len(pkCols))
			}
			var conds []string
			for pi, pk := range pkCols {
				val := vals[pi]
				args = append(args, spannerProtoToGo(val, declTypes[pkIndex[pi]]))
				conds = append(conds, quoteIdent(pk)+" = ?")
			}
			ors = append(ors, "("+strings.Join(conds, " AND ")+")")
		}
		where = "WHERE " + strings.Join(ors, " OR ")
	} else if len(ks.GetRanges()) > 0 {
		if len(pkCols) != 1 {
			return "", nil, status.Error(codes.Unimplemented, "range reads over composite primary keys are not supported by the simulator")
		}
		pk := pkCols[0]
		var ors []string
		for _, r := range ks.GetRanges() {
			if sc := r.GetStartClosed(); sc != nil && len(sc.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(sc.GetValues()[0], &sppb.Type{Code: sppb.TypeCode_STRING}))
				ors = append(ors, quoteIdent(pk)+" >= ?")
			}
			if so := r.GetStartOpen(); so != nil && len(so.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(so.GetValues()[0], &sppb.Type{Code: sppb.TypeCode_STRING}))
				ors = append(ors, quoteIdent(pk)+" > ?")
			}
			if ec := r.GetEndClosed(); ec != nil && len(ec.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(ec.GetValues()[0], &sppb.Type{Code: sppb.TypeCode_STRING}))
				ors = append(ors, quoteIdent(pk)+" <= ?")
			}
			if eo := r.GetEndOpen(); eo != nil && len(eo.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(eo.GetValues()[0], &sppb.Type{Code: sppb.TypeCode_STRING}))
				ors = append(ors, quoteIdent(pk)+" < ?")
			}
		}
		if len(ors) > 0 {
			where = "WHERE " + strings.Join(ors, " AND ")
		}
	}
	// ORDER BY the primary key so a read returns rows in key order, matching
	// Spanner's guarantee.
	order := " ORDER BY " + quoteIdent(pkCols[0])
	q := "SELECT " + colList + " FROM " + quoteIdent(table) + " " + where + order
	if limit := req.GetLimit(); limit > 0 {
		q += " LIMIT " + strconv.FormatInt(limit, 10)
	}
	return q, args, nil
}

func (s *spannerDataGRPC) Read(ctx context.Context, req *sppb.ReadRequest) (*sppb.ResultSet, error) {
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return nil, err
	}
	b, err := spannerBackendFor(dbName)
	if err != nil {
		return nil, err
	}
	q, args, err := spannerBuildReadQuery(b, req)
	if err != nil {
		return nil, err
	}
	res, err := spannerRunQuery(ctx, b, q, args)
	if err != nil {
		return nil, err
	}
	// Read requests project the requested columns in order; reorder the result
	// columns to match if the query selected * (which preserves declared order).
	res = spannerReorderReadColumns(req, res)
	rs := spannerShapeResultSet(res)
	if txnID, begun, err := spannerResolveTxn(dbName, req.GetTransaction()); err != nil {
		return nil, err
	} else if begun {
		rs.Metadata.Transaction = &sppb.Transaction{Id: txnID}
	}
	return rs, nil
}

func (s *spannerDataGRPC) StreamingRead(req *sppb.ReadRequest, stream sppb.Spanner_StreamingReadServer) error {
	ctx := stream.Context()
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return err
	}
	b, err := spannerBackendFor(dbName)
	if err != nil {
		return err
	}
	q, args, err := spannerBuildReadQuery(b, req)
	if err != nil {
		return err
	}
	res, err := spannerRunQuery(ctx, b, q, args)
	if err != nil {
		return err
	}
	res = spannerReorderReadColumns(req, res)
	txnID, begun, err := spannerResolveTxn(dbName, req.GetTransaction())
	if err != nil {
		return err
	}
	fields := make([]*sppb.StructType_Field, len(res.columns))
	for i, name := range res.columns {
		fields[i] = &sppb.StructType_Field{Name: name, Type: res.types[i]}
	}
	first := &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}},
	}
	if begun {
		first.Metadata.Transaction = &sppb.Transaction{Id: txnID}
	}
	if err := stream.Send(first); err != nil {
		return err
	}
	for _, row := range res.rows {
		chunk := &sppb.PartialResultSet{Values: row.GetValues()}
		if err := stream.Send(chunk); err != nil {
			return err
		}
	}
	return nil
}

// spannerReorderReadColumns projects a Read's result onto the requested columns
// in the requested order. The query always selects the requested columns
// already, so this is only needed when the query selected * (no explicit column
// list) to project down to the requested set.
func spannerReorderReadColumns(req *sppb.ReadRequest, res *spannerExecResult) *spannerExecResult {
	cols := req.GetColumns()
	if len(cols) == 0 || len(res.columns) == 0 {
		return res
	}
	// If the result columns already match the request order, nothing to do.
	match := len(cols) == len(res.columns)
	if match {
		for i, c := range cols {
			if res.columns[i] != c {
				match = false
				break
			}
		}
	}
	if match {
		return res
	}
	// Build a projection from the result's declared columns.
	idx := map[string]int{}
	for i, c := range res.columns {
		idx[c] = i
	}
	out := &spannerExecResult{columns: cols, types: make([]*sppb.Type, len(cols))}
	for i, c := range cols {
		if j, ok := idx[c]; ok {
			out.types[i] = res.types[j]
		} else {
			out.types[i] = &sppb.Type{Code: sppb.TypeCode_STRING}
		}
	}
	for _, row := range res.rows {
		nv := &structpb.ListValue{Values: make([]*structpb.Value, len(cols))}
		for i, c := range cols {
			if j, ok := idx[c]; ok && j < len(row.GetValues()) {
				nv.Values[i] = row.GetValues()[j]
			} else {
				nv.Values[i] = &structpb.Value{Kind: &structpb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}}
			}
		}
		out.rows = append(out.rows, nv)
	}
	return out
}

// ---------------------------------------------------------------------------
// Transactions
// ---------------------------------------------------------------------------

func (s *spannerDataGRPC) BeginTransaction(_ context.Context, req *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return nil, err
	}
	if _, err := spannerBackendFor(dbName); err != nil {
		return nil, err
	}
	id := generateUUID()
	readOnly := req.GetOptions().GetReadOnly() != nil
	spannerTxns.Put(id, spannerTxnState{Database: dbName, ReadOnly: readOnly})
	return &sppb.Transaction{Id: []byte(id)}, nil
}

func (s *spannerDataGRPC) Commit(_ context.Context, req *sppb.CommitRequest) (*sppb.CommitResponse, error) {
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return nil, err
	}
	b, err := spannerBackendFor(dbName)
	if err != nil {
		return nil, err
	}
	// Validate / retire the begun transaction if one was supplied. A single-use
	// commit (no begun id) is accepted as-is.
	if txID := req.GetTransactionId(); len(txID) > 0 {
		st, ok := spannerTxns.Get(string(txID))
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "invalid transaction id")
		}
		if st.Database != dbName {
			return nil, status.Error(codes.InvalidArgument, "transaction does not belong to this session's database")
		}
		defer spannerTxns.Delete(string(txID))
	}

	tx, err := b.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin commit tx: %v", err)
	}
	for _, m := range req.GetMutations() {
		if err := spannerApplyMutation(tx, m); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}
	return &sppb.CommitResponse{CommitTimestamp: timestamppb.Now()}, nil
}

func (s *spannerDataGRPC) Rollback(_ context.Context, req *sppb.RollbackRequest) (*emptypb.Empty, error) {
	txID := req.GetTransactionId()
	if len(txID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "transaction_id is required")
	}
	st, ok := spannerTxns.Get(string(txID))
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid transaction id")
	}
	dbName, err := spannerSessionDatabase(req.GetSession())
	if err != nil {
		return nil, err
	}
	if st.Database != dbName {
		return nil, status.Error(codes.InvalidArgument, "transaction does not belong to this session's database")
	}
	spannerTxns.Delete(string(txID))
	return &emptypb.Empty{}, nil
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// spannerApplyMutation applies one write mutation inside a SQLite transaction.
// Insert / InsertOrUpdate / Replace / Update / Delete map onto SQLite's own
// INSERT / INSERT OR REPLACE / UPDATE / DELETE statements; the table's primary
// key scopes UPDATE and DELETE.
func spannerApplyMutation(tx *sql.Tx, m *sppb.Mutation) error {
	if m == nil {
		return nil
	}
	switch op := m.Operation.(type) {
	case *sppb.Mutation_Insert:
		return spannerExecWrite(tx, op.Insert, "INSERT")
	case *sppb.Mutation_Update:
		return spannerExecWrite(tx, op.Update, "UPDATE")
	case *sppb.Mutation_InsertOrUpdate:
		return spannerExecWrite(tx, op.InsertOrUpdate, "INSERT OR REPLACE")
	case *sppb.Mutation_Replace:
		return spannerExecWrite(tx, op.Replace, "INSERT OR REPLACE")
	case *sppb.Mutation_Delete_:
		return spannerExecDelete(tx, op.Delete)
	}
	return status.Error(codes.InvalidArgument, "mutation carried no recognized operation")
}

// spannerExecWrite applies an Insert/Update/InsertOrUpdate/Replace mutation.
// Column types are looked up from the SQLite schema so proto values are coerced
// to the right Go type on the way in (notably INT64 → int64).
func spannerExecWrite(tx *sql.Tx, w *sppb.Mutation_Write, verb string) error {
	if w == nil {
		return status.Error(codes.InvalidArgument, "write mutation is empty")
	}
	table := w.GetTable()
	cols := w.GetColumns()
	values := w.GetValues()
	if table == "" {
		return status.Error(codes.InvalidArgument, "mutation.table is required")
	}
	declCols, declTypes, err := spannerTableColumnsFromTx(tx, table)
	if err != nil {
		return err
	}
	typeByName := map[string]*sppb.Type{}
	for i, n := range declCols {
		typeByName[n] = declTypes[i]
	}
	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = "?"
	}
	var stmt string
	switch verb {
	case "INSERT", "INSERT OR REPLACE":
		quoted := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = quoteIdent(c)
		}
		stmt = verb + " INTO " + quoteIdent(table) + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	case "UPDATE":
		setClauses := make([]string, len(cols))
		for i, c := range cols {
			setClauses[i] = quoteIdent(c) + " = ?"
		}
		pkCols, err := spannerPrimaryKeyColumnsFromTx(tx, table)
		if err != nil {
			return err
		}
		// UPDATE's WHERE must pin the row by primary key. The mutation supplies
		// column values in declared order; the PK columns are a subset, so build
		// the WHERE by mapping each PK back to its position in cols.
		colIndex := map[string]int{}
		for i, c := range cols {
			colIndex[c] = i
		}
		var whereConds []string
		for _, pk := range pkCols {
			pos, ok := colIndex[pk]
			if !ok {
				return status.Errorf(codes.InvalidArgument, "UPDATE mutation for %s does not specify primary key column %s", table, pk)
			}
			_ = pos
			whereConds = append(whereConds, quoteIdent(pk)+" = ?")
		}
		stmt = "UPDATE " + quoteIdent(table) + " SET " + strings.Join(setClauses, ", ") + " WHERE " + strings.Join(whereConds, " AND ")
	}

	for _, row := range values {
		vals := row.GetValues()
		if len(vals) != len(cols) {
			return status.Errorf(codes.InvalidArgument, "mutation row has %d values, expected %d", len(vals), len(cols))
		}
		args := make([]any, 0, len(vals)+len(cols))
		for i, c := range cols {
			args = append(args, spannerProtoToGo(vals[i], typeByName[c]))
		}
		if verb == "UPDATE" {
			// Append the PK values (already included in the SET positions) so
			// the WHERE placeholders bind to the same row's keys.
			colIndex := map[string]int{}
			for i, c := range cols {
				colIndex[c] = i
			}
			pkCols, _ := spannerPrimaryKeyColumnsFromTx(tx, table)
			for _, pk := range pkCols {
				pos := colIndex[pk]
				args = append(args, spannerProtoToGo(vals[pos], typeByName[pk]))
			}
		}
		if _, err := tx.Exec(stmt, args...); err != nil {
			return status.Errorf(codes.Internal, "apply %s on %s: %v", verb, table, err)
		}
	}
	return nil
}

// spannerExecDelete applies a Delete mutation scoped by its key set.
func spannerExecDelete(tx *sql.Tx, d *sppb.Mutation_Delete) error {
	if d == nil {
		return status.Error(codes.InvalidArgument, "delete mutation is empty")
	}
	table := d.GetTable()
	if table == "" {
		return status.Error(codes.InvalidArgument, "delete.table is required")
	}
	pkCols, err := spannerPrimaryKeyColumnsFromTx(tx, table)
	if err != nil {
		return err
	}
	declCols, declTypes, err := spannerTableColumnsFromTx(tx, table)
	if err != nil {
		return err
	}
	typeByName := map[string]*sppb.Type{}
	for i, n := range declCols {
		typeByName[n] = declTypes[i]
	}
	pkIndex := map[int]int{}
	for i, n := range declCols {
		for pi, pk := range pkCols {
			if n == pk {
				pkIndex[pi] = i
			}
		}
	}

	ks := d.GetKeySet()
	if ks == nil || ks.GetAll() {
		if _, err := tx.Exec("DELETE FROM " + quoteIdent(table)); err != nil {
			return status.Errorf(codes.Internal, "delete all from %s: %v", table, err)
		}
		return nil
	}
	if len(ks.GetKeys()) > 0 {
		var ors []string
		var args []any
		for _, key := range ks.GetKeys() {
			vals := key.GetValues()
			if len(vals) < len(pkCols) {
				return status.Errorf(codes.InvalidArgument, "delete key has %d values, expected %d", len(vals), len(pkCols))
			}
			var conds []string
			for pi, pk := range pkCols {
				args = append(args, spannerProtoToGo(vals[pi], typeByName[pkCols[pi]]))
				_ = pkIndex
				conds = append(conds, quoteIdent(pk)+" = ?")
			}
			ors = append(ors, "("+strings.Join(conds, " AND ")+")")
		}
		stmt := "DELETE FROM " + quoteIdent(table) + " WHERE " + strings.Join(ors, " OR ")
		if _, err := tx.Exec(stmt, args...); err != nil {
			return status.Errorf(codes.Internal, "delete from %s: %v", table, err)
		}
		return nil
	}
	if len(ks.GetRanges()) > 0 {
		if len(pkCols) != 1 {
			return status.Error(codes.Unimplemented, "range deletes over composite primary keys are not supported by the simulator")
		}
		pk := pkCols[0]
		var conds []string
		var args []any
		for _, r := range ks.GetRanges() {
			if sc := r.GetStartClosed(); sc != nil && len(sc.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(sc.GetValues()[0], typeByName[pk]))
				conds = append(conds, quoteIdent(pk)+" >= ?")
			}
			if so := r.GetStartOpen(); so != nil && len(so.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(so.GetValues()[0], typeByName[pk]))
				conds = append(conds, quoteIdent(pk)+" > ?")
			}
			if ec := r.GetEndClosed(); ec != nil && len(ec.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(ec.GetValues()[0], typeByName[pk]))
				conds = append(conds, quoteIdent(pk)+" <= ?")
			}
			if eo := r.GetEndOpen(); eo != nil && len(eo.GetValues()) == 1 {
				args = append(args, spannerProtoToGo(eo.GetValues()[0], typeByName[pk]))
				conds = append(conds, quoteIdent(pk)+" < ?")
			}
		}
		stmt := "DELETE FROM " + quoteIdent(table) + " WHERE " + strings.Join(conds, " AND ")
		if _, err := tx.Exec(stmt, args...); err != nil {
			return status.Errorf(codes.Internal, "delete range from %s: %v", table, err)
		}
		return nil
	}
	return nil
}

// spannerTableColumnsFromTx is the *sql.Tx variant of spannerTableColumns, so
// mutations running inside a Commit transaction see a consistent view.
func spannerTableColumnsFromTx(tx *sql.Tx, table string) ([]string, []*sppb.Type, error) {
	rows, err := tx.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, nil, status.Errorf(codes.NotFound, "table %q not found: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	var types []*sppb.Type
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, nil, status.Errorf(codes.Internal, "scan table_info: %v", err)
		}
		names = append(names, name)
		types = append(types, spannerTypeForSQLite(ctype))
	}
	if len(names) == 0 {
		return nil, nil, status.Errorf(codes.NotFound, "table not found: %s", table)
	}
	return names, types, nil
}

func spannerPrimaryKeyColumnsFromTx(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read table_info for %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	pkOrder := map[string]int{}
	maxPK := 0
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, status.Errorf(codes.Internal, "scan table_info: %v", err)
		}
		if pk > 0 {
			pkOrder[name] = pk
			if pk > maxPK {
				maxPK = pk
			}
		}
	}
	var pkCols []string
	for i := 1; i <= maxPK; i++ {
		for name, idx := range pkOrder {
			if idx == i {
				pkCols = append(pkCols, name)
			}
		}
	}
	if len(pkCols) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "table %s has no primary key", table)
	}
	return pkCols, nil
}

// ---------------------------------------------------------------------------
// Partitioning
// ---------------------------------------------------------------------------

// PartitionQuery / PartitionRead return a single empty partition. The simulator
// holds each database in one process, so the faithful partition of any query is
// a single unpartitioned run; the response shape is preserved so clients that
// fan out across partitions still function.
func (s *spannerDataGRPC) PartitionQuery(_ context.Context, req *sppb.PartitionQueryRequest) (*sppb.PartitionResponse, error) {
	return spannerSinglePartition(req.GetSession()), nil
}

func (s *spannerDataGRPC) PartitionRead(_ context.Context, req *sppb.PartitionReadRequest) (*sppb.PartitionResponse, error) {
	return spannerSinglePartition(req.GetSession()), nil
}

func spannerSinglePartition(session string) *sppb.PartitionResponse {
	tok := []byte(session + "-p0")
	return &sppb.PartitionResponse{
		Partitions: []*sppb.Partition{{PartitionToken: tok}},
	}
}

// ExecuteBatchDml, BatchWrite, and ExecuteAction remain on the
// UnimplementedSpannerServer default. They are outside the high-level client's
// Apply / Read / Query / ReadWriteTransaction paths exercised here, and shipping
// a partial or synthetic implementation would violate the no-stubs rule.
