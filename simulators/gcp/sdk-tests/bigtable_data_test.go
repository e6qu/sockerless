package gcp_sdk_test

import (
	"testing"

	"cloud.google.com/go/bigtable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBigtable_DataPlane exercises the Cloud Bigtable data API (BUG-2159) via the
// official `cloud.google.com/go/bigtable` client over gRPC (reached through
// BIGTABLE_EMULATOR_HOST — the same coordinate the client uses for the real
// emulator): MutateRow, ReadRow/ReadRows with a RowFilter, ReadModifyWrite
// increment, CheckAndMutateRow, a row-range scan, and DeleteFromRow.
func TestBigtable_DataPlane(t *testing.T) {
	t.Setenv("BIGTABLE_EMULATOR_HOST", grpcAddr)
	const (
		project  = "bt-data-proj"
		instance = "bt-data-inst"
		table    = "events"
		family   = "cf"
	)

	iac, err := bigtable.NewInstanceAdminClient(ctx, project)
	require.NoError(t, err)
	defer iac.Close()
	require.NoError(t, iac.CreateInstance(ctx, &bigtable.InstanceConf{
		InstanceId:  instance,
		DisplayName: instance,
		ClusterId:   "c1",
		Zone:        "us-east1-b",
		NumNodes:    1,
	}))

	ac, err := bigtable.NewAdminClient(ctx, project, instance)
	require.NoError(t, err)
	defer ac.Close()
	require.NoError(t, ac.CreateTable(ctx, table))
	require.NoError(t, ac.CreateColumnFamily(ctx, table, family))

	client, err := bigtable.NewClient(ctx, project, instance)
	require.NoError(t, err)
	defer client.Close()
	tbl := client.Open(table)

	// MutateRow (SetCell) + ReadRow.
	mut := bigtable.NewMutation()
	mut.Set(family, "name", bigtable.Now(), []byte("alice"))
	require.NoError(t, tbl.Apply(ctx, "user#1", mut))

	row, err := tbl.ReadRow(ctx, "user#1")
	require.NoError(t, err)
	require.Len(t, row[family], 1)
	assert.Equal(t, family+":name", row[family][0].Column)
	assert.Equal(t, "alice", string(row[family][0].Value))

	// Multiple versions + LatestNFilter(1) keeps only the newest cell.
	for _, v := range []string{"v1", "v2", "v3"} {
		m := bigtable.NewMutation()
		m.Set(family, "ver", bigtable.Now(), []byte(v))
		require.NoError(t, tbl.Apply(ctx, "user#1", m))
	}
	row, err = tbl.ReadRow(ctx, "user#1", bigtable.RowFilter(bigtable.ChainFilters(
		bigtable.ColumnFilter("ver"), bigtable.LatestNFilter(1))))
	require.NoError(t, err)
	require.Len(t, row[family], 1, "LatestNFilter(1) must keep one cell")
	assert.Equal(t, "v3", string(row[family][0].Value))

	// ReadModifyWrite increment.
	rmw := bigtable.NewReadModifyWrite()
	rmw.Increment(family, "count", 5)
	if _, err := tbl.ApplyReadModifyWrite(ctx, "user#1", rmw); err != nil {
		t.Fatalf("ApplyReadModifyWrite: %v", err)
	}
	rmw2 := bigtable.NewReadModifyWrite()
	rmw2.Increment(family, "count", 7)
	after, err := tbl.ApplyReadModifyWrite(ctx, "user#1", rmw2)
	require.NoError(t, err)
	var count int64
	for _, ri := range after[family] {
		if ri.Column == family+":count" {
			count = int64(bigEndian(ri.Value))
		}
	}
	assert.Equal(t, int64(12), count, "increment 5 then 7 = 12")

	// CheckAndMutateRow: predicate matches an existing cell → apply trueMutation.
	trueMut := bigtable.NewMutation()
	trueMut.Set(family, "verified", bigtable.Now(), []byte("yes"))
	cond := bigtable.NewCondMutation(bigtable.ColumnFilter("name"), trueMut, nil)
	require.NoError(t, tbl.Apply(ctx, "user#1", cond))
	row, err = tbl.ReadRow(ctx, "user#1", bigtable.RowFilter(bigtable.ColumnFilter("verified")))
	require.NoError(t, err)
	require.Len(t, row[family], 1)
	assert.Equal(t, "yes", string(row[family][0].Value))

	// Row-range scan: write a few keys, scan [user#1, user#3).
	for _, k := range []string{"user#2", "user#3"} {
		m := bigtable.NewMutation()
		m.Set(family, "name", bigtable.Now(), []byte(k))
		require.NoError(t, tbl.Apply(ctx, k, m))
	}
	var scanned []string
	err = tbl.ReadRows(ctx, bigtable.NewRange("user#1", "user#3"), func(r bigtable.Row) bool {
		scanned = append(scanned, r.Key())
		return true
	}, bigtable.RowFilter(bigtable.ColumnFilter("name")))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user#1", "user#2"}, scanned, "[user#1,user#3) excludes user#3")

	// DeleteFromRow removes the row entirely.
	del := bigtable.NewMutation()
	del.DeleteRow()
	require.NoError(t, tbl.Apply(ctx, "user#2", del))
	row, err = tbl.ReadRow(ctx, "user#2")
	require.NoError(t, err)
	assert.Nil(t, row, "deleted row must read back empty")
}

func bigEndian(b []byte) uint64 {
	var n uint64
	for _, x := range b {
		n = n<<8 | uint64(x)
	}
	return n
}
