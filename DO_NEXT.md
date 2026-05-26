# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

Main is idle after PR #231, the issue #230 Service Bus raw AMQP/TLS transport fix. Azure Service Bus now exposes the official SDK's default AMQP/TLS path in addition to the WebSocket AMQP path from PR #229.

## Stage plan

Current phase: none. Start the next pass by syncing `main`, listing open GitHub issues, filing each real issue in `BUGS.md`, then creating a fresh branch from `origin/main`.

Issue #227 finding: Blob had single-shot Put/Get coverage but no block-list subresource dispatch, so `?comp=block` and `?comp=blocklist` were misrouted. The fix persists committed and uncommitted block state and materializes committed block blobs in list order.

Issue #228 finding: Service Bus REST data-plane coverage did not cover the official Go SDK, which uses AMQP 1.0 over WebSocket. The fix adds the AMQP slice for SASL anonymous, CBS claim RPC, entity sender/receiver links, link credit, accepted dispositions, receive-and-delete transfers, topic fan-out, and subscription receiver paths.

Issue #230 finding: WebSocket-only AMQP support still leaked simulator transport plumbing into public simulator callers because official SDK users had to provide `NewWebSocketConn`. The fix adds a raw AMQP/TLS listener, derives namespace from protocol-visible host data, and covers queue plus topic/subscription Send/Receive with the official SDK using `CustomEndpoint` and no WebSocket adapter.

Issue #230 plan/contract:
- Raw AMQP/TLS is a separate Service Bus listener because it is a TCP/TLS transport, not an HTTP path route.
- Namespace routing prefers AMQP Open `hostname`, with TLS SNI as early metadata/fallback. This matches the SDK `CustomEndpoint` model: redirect the TCP target while preserving the original Service Bus namespace/audience.
- Queue, topic, subscription, CBS, and management routing use AMQP link source/target addresses: `{queue}`, `{topic}`, `{topic}/Subscriptions/{subscription}`, `$cbs`, and `{entity}/$management`.
- No simulator-only `/namespace/...` path routing exists for raw AMQP; that would be less cloud-faithful and would leak simulator plumbing into clients.
- The canonical regressions use the official `azservicebus` SDK default AMQP/TLS path with `ClientOptions.CustomEndpoint` and TLS config, without `NewWebSocketConn`, for both queue and topic/subscription Send/Receive.

## Standing invariants (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs in BUGS.md *before* any fix attempt.
- No fakes / no fallbacks / no silent shims.
- Every reopen carries a postmortem trail (`.claude/skills/reopen-postmortem/SKILL.md`).
- Every closed-enumeration surface has a `specs/SIM_SURFACE_TABLES/<name>.md` with no silent ✗ rows.
- Every List* op has a paged-iterator test (`sim-canonical-config-test` rule).
- Every stateful resource type has a state-machine assertion (`sim-state-machine-completeness`).
- `make hooks` on every fresh clone (wires `mux-overlap-scan` + gofmt + golangci-lint + …).

## Session-resume checklist

1. `git fetch origin && gh pr list --state open && git status`.
2. If a phase PR is open: `gh pr checks <N>`; report state.
3. If a PR merged: sync `main`, delete merged branch, prune remotes, refresh continuity docs to idle.
4. If fresh issues filed: `gh issue list --state open --limit 30`; file each as a BUG in BUGS.md before any fix attempt.
5. Read `.claude/skills/avoid-vibe-slop/SKILL.md` before any code change.

## Reference for next reopen / new issue

If a community-filed issue surfaces against a closed enumeration (subresources, ops on a single service, paged List, state-bearing resource), the routine is:

1. Identify the surface table at `specs/SIM_SURFACE_TABLES/<surface>.md`. If none exists, create one before any fix.
2. File a BUG in BUGS.md citing which row(s) the issue covers + which siblings should be checked.
3. Fix the named row + every reasonable sibling (`surface-table-completeness` rule).
4. SDK test uses the canonical client (no raw `net/http` where an SDK exists; for List* use a Pager).
5. For reopens: BUG entry MUST include the three postmortem fields (what test passed but should have failed / what SDK code path was missed / what new canonical-client test catches the regression).
