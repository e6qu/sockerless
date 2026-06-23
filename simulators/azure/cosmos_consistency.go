package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Cosmos consistency-level validation + session-token issuance.
//
// Consistency level: a data-plane request may carry an `x-ms-consistency-level`
// header (Strong/BoundedStaleness/Session/ConsistentPrefix/Eventual) to override
// the account's default for that operation. Real Cosmos rejects a request that
// asks for a level STRONGER than the account's configured maximum with a 400
// BadRequest — you cannot escalate past the account ceiling. The account's
// default/maximum is the consistencyPolicy.defaultConsistencyLevel surfaced by
// the ARM account resource and the account-discovery (`GET /`) response.
//
// Session tokens: under Session consistency (the default) every write returns an
// `x-ms-session-token` whose per-partition LSN advances monotonically; a Session
// read may send that token back, and the read observes at least that version
// (read-your-writes). The sim issues a monotonic per-(collection,partition) LSN
// and echoes a token of the form "<pkRangeId>:<lsn>" on writes, faithfully
// matching the azcosmos ItemResponse.SessionToken contract.

// cosmosConsistencyRank orders the consistency levels from weakest (0) to
// strongest. A request must not ask for a level ranked above the account max.
var cosmosConsistencyRank = map[string]int{
	"eventual":         0,
	"consistentprefix": 1,
	"session":          2,
	"boundedstaleness": 3,
	"strong":           4,
}

// cosmosAccountMaxConsistency returns the account's configured maximum
// consistency level for the data-plane account, defaulting to Session (the real
// Cosmos default) when no ARM account row declares one. The data plane is keyed
// by account name; the ARM account row (if created) carries the authoritative
// consistencyPolicy.
func cosmosAccountMaxConsistency(account string) string {
	for _, a := range cosmosAccounts.List() {
		if a.Name != account {
			continue
		}
		if cp, ok := a.Properties["consistencyPolicy"].(map[string]any); ok {
			if lvl, ok := cp["defaultConsistencyLevel"].(string); ok && lvl != "" {
				return lvl
			}
		}
	}
	return "Session"
}

// cosmosCheckConsistency validates an optional x-ms-consistency-level request
// header against the account maximum. It returns a write error (400) when the
// request asks for a level stronger than the account permits, mirroring real
// Cosmos. An absent or recognized-and-permitted header passes.
func cosmosCheckConsistency(r *http.Request, account string) *cosmosWriteError {
	requested := strings.TrimSpace(r.Header.Get("x-ms-consistency-level"))
	if requested == "" {
		return nil
	}
	reqRank, ok := cosmosConsistencyRank[strings.ToLower(requested)]
	if !ok {
		return &cosmosWriteError{
			code:   "BadRequest",
			msg:    fmt.Sprintf("Invalid consistency level %q.", requested),
			status: http.StatusBadRequest,
		}
	}
	maxLevel := cosmosAccountMaxConsistency(account)
	maxRank := cosmosConsistencyRank[strings.ToLower(maxLevel)]
	if reqRank > maxRank {
		return &cosmosWriteError{
			code: "BadRequest",
			msg: fmt.Sprintf(
				"Requested consistency level %q cannot be satisfied: the account's maximum consistency level is %q.",
				requested, maxLevel),
			status: http.StatusBadRequest,
		}
	}
	return nil
}

// cosmosGuardConsistency validates the consistency header and writes the 400
// error if it fails, returning true when the caller should stop. Item handlers
// call this at entry so an unsupported escalation is rejected faithfully.
func cosmosGuardConsistency(w http.ResponseWriter, r *http.Request, account string) bool {
	if werr := cosmosCheckConsistency(r, account); werr != nil {
		cosmosDataError(w, werr.code, werr.msg, werr.status)
		return true
	}
	return false
}

// ── session tokens ───────────────────────────────────────────────────────────

// cosmosPartitionLSNs holds the monotonic per-partition LSN for session-token
// issuance, keyed by (account,db,coll,partition-key-component).
var (
	cosmosPartitionLSNs   sync.Map // string -> *atomic.Uint64
	cosmosPKRangeAssign   sync.Map // string (account/db/coll/pk) -> int (stable range id)
	cosmosPKRangeNextID   atomic.Uint64
	cosmosPKRangeAssignMu sync.Mutex
)

func cosmosSessionPartitionKey(account, db, coll, pkComponent string) string {
	return account + "/" + db + "/" + coll + "/" + pkComponent
}

// cosmosPKRangeID returns a stable small integer partition-key-range id for a
// (collection, partition) pair. Real session tokens are "<pkRangeId>:<lsn>"; the
// range id groups logical partitions, but a stable per-partition id is a faithful
// observable for a single-node sim (each logical partition maps to its own range
// deterministically).
func cosmosPKRangeID(key string) int {
	if v, ok := cosmosPKRangeAssign.Load(key); ok {
		if id, ok := v.(int); ok {
			return id
		}
	}
	cosmosPKRangeAssignMu.Lock()
	defer cosmosPKRangeAssignMu.Unlock()
	if v, ok := cosmosPKRangeAssign.Load(key); ok {
		if id, ok := v.(int); ok {
			return id
		}
	}
	id := int(cosmosPKRangeNextID.Add(1) - 1)
	cosmosPKRangeAssign.Store(key, id)
	return id
}

// cosmosAdvanceSessionLSN bumps and returns the per-partition LSN for a write.
func cosmosAdvanceSessionLSN(key string) uint64 {
	v, _ := cosmosPartitionLSNs.LoadOrStore(key, &atomic.Uint64{})
	c, ok := v.(*atomic.Uint64)
	if !ok {
		return 0
	}
	return c.Add(1)
}

// cosmosCurrentSessionLSN returns the current per-partition LSN (0 if no write
// has happened yet) without advancing it.
func cosmosCurrentSessionLSN(key string) uint64 {
	v, ok := cosmosPartitionLSNs.Load(key)
	if !ok {
		return 0
	}
	c, ok := v.(*atomic.Uint64)
	if !ok {
		return 0
	}
	return c.Load()
}

// cosmosSessionToken formats a "<pkRangeId>:<lsn>" session token.
func cosmosSessionToken(rangeID int, lsn uint64) string {
	return strconv.Itoa(rangeID) + ":" + strconv.FormatUint(lsn, 10)
}

// cosmosSetWriteSession advances the partition LSN for a write and sets the
// x-ms-session-token response header the azcosmos ItemResponse surfaces.
func cosmosSetWriteSession(w http.ResponseWriter, account, db, coll, pkComponent string) {
	key := cosmosSessionPartitionKey(account, db, coll, pkComponent)
	lsn := cosmosAdvanceSessionLSN(key)
	w.Header().Set("x-ms-session-token", cosmosSessionToken(cosmosPKRangeID(key), lsn))
}

// cosmosEchoReadSession sets the x-ms-session-token on a read response. Under
// Session consistency a read returns the current partition LSN (at least the
// version the client's supplied token requested), enabling read-your-writes.
func cosmosEchoReadSession(w http.ResponseWriter, r *http.Request, account, db, coll, pkComponent string) {
	key := cosmosSessionPartitionKey(account, db, coll, pkComponent)
	lsn := cosmosCurrentSessionLSN(key)
	// A client-supplied session token requests at least its LSN; the read reflects
	// the max of the client token and the current partition LSN.
	if client := r.Header.Get("x-ms-session-token"); client != "" {
		if _, clientLSN, ok := cosmosParseSessionToken(client); ok && clientLSN > lsn {
			lsn = clientLSN
		}
	}
	w.Header().Set("x-ms-session-token", cosmosSessionToken(cosmosPKRangeID(key), lsn))
}

// cosmosParseSessionToken parses a "<pkRangeId>:<lsn>" token.
func cosmosParseSessionToken(tok string) (rangeID int, lsn uint64, ok bool) {
	i := strings.LastIndexByte(tok, ':')
	if i < 0 {
		return 0, 0, false
	}
	rid, err1 := strconv.Atoi(tok[:i])
	l, err2 := strconv.ParseUint(tok[i+1:], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return rid, l, true
}
