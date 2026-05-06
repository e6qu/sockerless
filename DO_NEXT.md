# Do Next

Resume pointer for next session. Detail: [STATUS.md](STATUS.md), [PLAN.md](PLAN.md), [WHAT_WE_DID.md](WHAT_WE_DID.md), [BUGS.md](BUGS.md).

## Today's outcome (2026-05-06)

**6/8 cells GREEN** (cells 1–4 from earlier phases + cells 7+8 today). Cells 5+6 (GH × GCP) still failing — peeled 5 architectural fixes (BUG-956/957/958/959/960) landing them past materialize/persist; surfaced 2 more layered architectural blockers (BUG-961/962) that need a fresh session to fix.

Code on `phase-118-faas-pods @ e8a85e6` (pushed). Live infra `sockerless-live-46x3zg4imo` is up.

## Resume sequence

### Step 1 — BUG-962: gcf exec response stream framing

Cell 6 v4 produced the failure mode: `Unrecognized input header: 115` (byte = `'s'` from `sockerless-...` stderr text). gcf's `execStartViaInvoke` returns plain bytes; docker exec non-TTY response needs 8-byte stream-frame headers `[stream_type, 0, 0, 0, len_be32]` per chunk.

```go
// backends/cloudrun-functions/exec_invoke.go (and same shape in cloudrun/)
// after `res, err := gcpcommon.PostExecEnvelope(...)`:
var buf bytes.Buffer
core.WriteFrame(&buf, core.StreamStdout, res.Stdout)
core.WriteFrame(&buf, core.StreamStderr, res.Stderr)
return readOnlyRWC(buf.Bytes()), nil
```

Find `core.WriteFrame` (or equivalent existing helper in `backends/core/`); reverse-agent path frames the same way — mirror it.

### Step 2 — BUG-961: cloudrun exec POST hang

Cell 5 v4 hung 10 min. Pod-Service materialized fine, bootstrap listening on :8080, but the envelope POST never reached the bootstrap (no `handleInvoke` log entry on the receiving side).

Investigation steps:
1. Add `ENTRY` log at the top of cloudrun bootstrap's `handleInvoke` so we see whether the POST arrives at all.
2. Re-check `serviceInvokeURL` — make sure it returns the URL of the just-materialized Service, not a stale one.
3. Check that `idtoken.NewClient(audience=URL)` uses the same audience as the Service URL (a mismatch silently 401s).
4. Verify VPC connector + ingress: the cloudrun runner-task is a Cloud Run JOB; can it reach a Cloud Run Service URL? It should via the VPC connector.

Note: cell 5 likely also has BUG-962 once BUG-961 is resolved.

### Step 3 — Re-trigger cells 5+6

The PR #124 trigger (`pull_request: paths: [<cell-yml>]`) fires BOTH workflows on every push because the cumulative PR diff includes both files. Quota contention with both running in parallel is a known issue (BUG-942 family). Mitigations:

- Aggressive cleanup before each trigger (delete `sockerless-svc-*` + `skls-*` + cancel stale `gh-*` Cloud Run Job executions).
- Or: temporarily edit one workflow's `runs-on:` to a label that doesn't match (force only the other to fire).

```bash
# Cleanup recipe
for svc in $(gcloud run services list --project=sockerless-live-46x3zg4imo --region=us-central1 \
    --format='value(metadata.name)' | grep -E '^(sockerless-svc-|skls-)'); do
  gcloud run services delete "$svc" --project=sockerless-live-46x3zg4imo --region=us-central1 --quiet
done
for exec in $(gcloud run jobs executions list --project=sockerless-live-46x3zg4imo --region=us-central1 \
    --format='value(metadata.name)' | grep '^gh-'); do
  gcloud run jobs executions cancel "$exec" --project=sockerless-live-46x3zg4imo --region=us-central1 --quiet
done

# Trigger via PR #124
git fetch origin cell-workflows-on-main
git worktree add -B pr124 /tmp/pr124 origin/cell-workflows-on-main
cd /tmp/pr124/ui && bun install && cd /Users/zardoz/projects/sockerless
git -C /tmp/pr124 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 5+6 — re-trigger after BUG-961+962/" /tmp/pr124/.github/workflows/cell-5-cloudrun.yml
sed -i '' "1s/.*/# Cell 5+6 — re-trigger after BUG-961+962/" /tmp/pr124/.github/workflows/cell-6-gcf.yml
git -C /tmp/pr124 add .github/workflows/cell-5-cloudrun.yml .github/workflows/cell-6-gcf.yml
git -C /tmp/pr124 commit -m "trigger: cells 5+6 v5 after BUG-961+962"
git -C /tmp/pr124 push origin pr124:cell-workflows-on-main
git -C /Users/zardoz/projects/sockerless worktree remove /tmp/pr124 --force
git -C /Users/zardoz/projects/sockerless branch -D pr124
```

### Step 4 — Closeout

After cells 5+6 GREEN:
1. Update STATUS.md cell scoreboard (mark 5+6 ✅).
2. Update WHAT_WE_DID.md with cells 5+6 outcome.
3. Update PR #123 description to reflect all 8 cells GREEN.
4. State save commit. NEVER MERGE — user handles merges.

## Already-pushed assets (carried into resume)

Backend digests deployed:
- `sockerless-backend-cloudrun@sha256:a221956c` (cell 7 v54 GREEN)
- `sockerless-backend-gcf@sha256:d792e563` (cell 8 v28 GREEN)

GH runner-task images (carry today's BUG-957/958/959/960 fixes):
- `runner:cloudrun-amd64@sha256:718e78ad`
- `runner:gcf-amd64@sha256:451eed70`

These will need rebuilds + push after BUG-961/962 fixes land — `make -C tests/runners/github/dockerfile-{cloudrun,gcf} push-amd64`.

## Single-line summary

> 6/8 cells GREEN (1–4 + 7 + 8). Cells 5+6 progressed past 4 architectural blockers today (BUG-956→960). Two new exec-path bugs (BUG-961 cloudrun POST hang, BUG-962 gcf stream framing) need fresh-session attention. Live project still up.
