# Pod-Model Analysis Across the 7 Backends

**Phase 167 — analysis only.** No code edits.

Sockerless presents a Docker REST API. Behind it sit 7 backends with very different lifecycle primitives. This doc compares the "pod" abstraction across all 7, traces how GitHub Actions + GitLab runners hit each, identifies a load-bearing per-step performance footgun (the suspected "12 steps took 12+ min" case), and proposes simplifications.

Goals (per the request that opened the phase):
- Compare the pod abstraction side-by-side across backends.
- Identify simplification opportunities for FaaS backends.
- Avoid "exotic" storage options by default (keep them available, but don't default to them).
- No separate-VM/instance hacks — every backend stays on its native primitive.
- Root-cause a CI job that took 12+ min for 12 steps (~1 min initialization per step).

## TL;DR

1. **The pod abstraction is NOT uniform.** Long-lived backends (docker/ecs/cloudrun/aca) hold one container/task/revision for the whole CI job and route per-step `docker exec` directly. FaaS backends (lambda/gcf/azf) are invoke-on-demand: they create a *function* per logical pod, then dispatch each step either through Path A (reverse-agent WebSocket — fast, one warm execution-environment) or Path B (fresh per-exec `Invoke` — pays cold-start every step). **Path B is the suspected cause of the "12 steps = 12 min" symptom.**
2. **Storage defaults are mostly correct.** GCP's `gcs-sync` (per-exec tar/untar) is the right default — `pd-ephemeral` is exotic and stays opt-in. Azure has only `azure-files-ephemeral` available. AWS has only `efs-ephemeral` for Lambda. No exotic-by-default to remove; the existing pattern is sound.
3. **One real simplification:** force Path A as the only exec path for FaaS — drop Path B from the default flow, OR refuse to start a container until the reverse-agent has dialed back. This makes the per-step cost the cost of one HTTP round-trip on a warm WebSocket (~10 ms) instead of one Lambda cold-start (~1 min).

## 1. Pod abstraction per backend

| Backend | Pod = | Created via | Step model | Default storage | Job-lifetime mapping |
|---|---|---|---|---|---|
| **docker** | Local container | `docker create` | reuse via `docker exec` | host bind-mount (none) | 1 container for whole job |
| **ecs** | Fargate task | `ecs.RunTask` | per-step `ecs.RunTask` sub-tasks (siblings of runner task) | `efs-ephemeral` | 1 runner task + N sub-tasks |
| **lambda** | Lambda function (image mode + overlay) | `lambda.CreateFunction` | **Path A** WS exec OR **Path B** fresh `Invoke` per `docker exec` | `efs-ephemeral` | 1 runner-Lambda; each step's `docker create` makes a NEW sub-Lambda from pool |
| **cloudrun** | Cloud Run Service revision | `run.Services.CreateService` + `CreateRevision` | per-step new revision; pre/post tar/untar to GCS | `gcs-sync` (exotic `pd-ephemeral` opt-in) | 1 Service, many revisions |
| **gcf** | Cloud Functions Gen2 (underlying Cloud Run Service) | `functions.CreateFunction` + `run.Services.UpdateService` escape hatch | per-`docker create`: pool query → cache hit reuse OR fresh function | `gcs-sync` (exotic `pd-ephemeral` opt-in) | Pool-reusable functions; 1 logical job potentially across multiple functions |
| **aca** | Container Apps app revision | `armappcontainers.ContainerAppsClient.CreateOrUpdate` | per-step new revision; **persistent Azure Files mount** (no tar/untar) | `azure-files-ephemeral` | 1 App, many revisions |
| **azf** | Function App (Linux Flex Consumption) | `armappservice.WebAppsClient.BeginCreateOrUpdate` | per-`docker create`: new Function App OR supervisor-overlay if multi-pod | `azure-files-ephemeral` | 1 Function App per `docker create` (potentially many per job) |

### What "uniform" actually means

- **docker / ecs / cloudrun / aca:** the runner's intuition holds — one cloud primitive holds the workspace, all step execs land on it (or peer-task siblings that share storage). Cold-start cost is paid once per job, not once per step.
- **lambda / gcf / azf:** the runner's `docker create` doesn't map to "boot a long-lived box." It maps to "stand up a function." Per-step `docker exec` ONLY stays cheap if the bootstrap inside the function dials back via WebSocket (Path A). Without that, each step pays a fresh-function-invoke tax.

## 2. Runner call sequence

### GitHub Actions

```
docker create  → 1   (per `container:` block — usually 1)
docker pull    → 1   (cached on the runner host after first job)
docker start   → 1
docker attach  → 1
docker exec    → N   (1 per step)
docker wait    → 1
docker rm      → 1
```

**Contract:** *one container per job, exec per step.* GH Actions does NOT recreate a container between steps. It DOES expect each `docker exec` to behave like a fresh shell session that sees the workspace state from the previous step.

### GitLab Runner (docker executor)

```
docker create  → 1   (helper container, attach-before-start pattern)
docker pull    → 1
docker attach  → 1
docker start   → 1   (start cycle 1)
   → stdin-pipe script for "prepare" stage
docker wait    → 1   (cycle 1 exit)
docker start   → 1   (start cycle 2)   (BUG-related second start — see backend_impl.go:507 comment)
   → stdin-pipe script for "build" stage
docker wait    → 1
docker start   → 1   (start cycle 3)
   → stdin-pipe script for "post-build" stage
docker wait    → 1
docker rm      → 1
```

**Contract:** *one container, multiple "start cycles."* Each cycle = one stage with its own stdin script. GitLab does NOT use `docker exec`; it uses `attach` + multiple `start` cycles with hijacked stdin.

For FaaS, both runner contracts map to per-step *function dispatch* — for GH that's an exec, for GitLab that's a start cycle. **In both cases, the cost is the same if Path A (reverse-agent) is in play, and pathological if Path B (fresh Invoke per step/cycle) is.**

## 3. Driver matrix (network-discovery / dns / access / storage)

| Backend | network-discovery | dns | access | storage default | exotic alternatives |
|---|---|---|---|---|---|
| docker | host-aliases | none | none-internal | memory + host bind | — |
| ecs | service-mesh | cloud-map | iam-role (SigV4) | **efs-ephemeral** | — (no other persistent AWS primitive on ECS Fargate) |
| lambda | nat-gateway-only (or host-aliases) | none (or cloud-map) | iam-role | **efs-ephemeral** | — (Lambda has no other persistent primitive in invoke ENV) |
| cloudrun | cloud-dns-zone (or host-aliases) | cloud-dns-zone | id-token | **gcs-sync** | `pd-ephemeral` (regional PD attach, quota-limited; slower than tmpfs+tar for typical CI workspaces) |
| gcf | host-aliases | none | id-token | **gcs-sync** | `pd-ephemeral` (same caveats as Cloud Run) |
| aca | cloud-dns (private-dns-zone) | private-dns-zone | none-internal (or azure-ad) | **azure-files-ephemeral** | — (ACA has no native ephemeral disk mount) |
| azf | nat-gateway-only (or host-aliases / cloud-dns) | private-dns-zone | none-internal (or azure-ad) | **azure-files-ephemeral** | — (AZF has no native ephemeral disk mount) |

### Storage assessment

- **AWS (ECS+Lambda):** `efs-ephemeral` is the only choice. Not exotic. Keep.
- **GCP (Cloud Run+GCF):** `gcs-sync` default is correct. `pd-ephemeral` is exotic (regional PD attach quota + slower per-step than tar/untar for typical workspaces). **Already opt-in** — operator must explicitly select. No change needed.
- **Azure (ACA+AZF):** `azure-files-ephemeral` is the only choice. Not exotic. Keep.

**Conclusion on "exotic-by-default":** there isn't really an exotic default to fix. Each cloud backend defaults to the only realistic option its platform exposes for persistent cross-step storage. `pd-ephemeral` is the one exotic primitive in the registry and it's already opt-in, not default.

## 4. The "12 steps = 12 min" root cause

### Hypothesis (load-bearing)

The user observed a 12-step CI job taking ~12+ min with ~1 min initialization per step. The pattern matches **Lambda's Path B (fresh `Invoke` per `docker exec`)** being taken instead of Path A (reverse-agent WebSocket).

Smoking-gun code, `backends/lambda/backend_delegates.go:201`:

```go
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
    ...
    if _, hasAgent := s.reverseAgents.Resolve(c.ID); hasAgent {
        return s.BaseServer.ExecStart(id, opts)  // Path A (WS — fast)
    }
    return s.execStartViaInvoke(id, exec, opts)  // Path B (fresh Invoke — cold-start)
}
```

The same Path A/B split exists in `backends/cloudrun-functions/backend_delegates.go:216-223` (gcf) and `backends/azure-functions/backend_delegates.go:203` (azf).

When the reverse-agent inside the container hasn't dialed back yet (or didn't dial back at all), every `docker exec` becomes a fresh `lambda.Invoke`. Image-based Lambdas with EFS + VPC ENI attach cold-start in **30–90 sec** (ENI attach alone is ~10 sec; image pull + bootstrap is the rest). For a 12-step job, 12 cold-starts ≈ 12 min — exactly the symptom reported.

### Path A failure modes (why the reverse-agent might not be there)

Most likely (in order):

1. **Bootstrap dial-back hasn't completed by the time the first `docker exec` arrives.** GH Actions issues `docker create` → `docker start` → `docker exec` rapidly. The bootstrap's WebSocket dial may not be registered yet when `ExecStart` checks `reverseAgents.Resolve`.
2. **`SOCKERLESS_CALLBACK_URL` not reachable from inside the function.** If the lambda is in a VPC without NAT to the sockerless control plane, the dial-back fails silently. The function still serves Path B execs via `Invoke`.
3. **The runner image doesn't include the `sockerless-lambda-bootstrap` overlay.** If the image was pulled directly (not via sockerless's overlay-injection path), there's no dial-back at all.
4. **Reverse-agent connection drops mid-job** and isn't re-established. After `docker exec #5`, the function goes idle, Lambda recycles the ENI, the WS times out; exec #6 hits Path B.

### Smaller contributors (also adding seconds per step)

- `ContainerStart` polls `FunctionActiveV2` waiter on every call. Fine on cycle 1 (function just created). On GitLab cycle 2+ it shouldn't need to wait again but does — `backends/lambda/backend_impl.go:603`.
- GitLab cycles 2+ go through `ResolveContainerAuto` fallback (`backend_impl.go:507-516`), which queries cloud state — another ~1–3 sec per cycle.
- Per-step stdin-pipe polling window (`backend_impl.go:640-650`) — up to 5 sec per cycle if `OpenStdin` is set and the runner is slow to attach.

### Verification path (next phase — not this one)

1. Add an INFO-level log line at `backend_delegates.go:210` that prints `path=A` or `path=B` for every ExecStart.
2. Run a 12-step CI workflow against Lambda backend, count Path A vs Path B occurrences.
3. If majority are Path B, capture timing per Invoke; cross-check against Lambda cold-start metrics in CloudWatch.

## 5. Simplification proposals

### 5.1 FaaS: make Path A mandatory (or near-mandatory)

The whole point of the bootstrap dial-back is to keep the function warm for the rest of the job. Two options:

- **(a) Block `ContainerStart` until the reverse-agent has dialed back.** Concretely: after `lambda.Invoke` of the bootstrap, poll `s.reverseAgents.Resolve(c.ID)` for up to N seconds. If it doesn't show up, fail the container with a clear error (operator: "your runner image is missing `sockerless-lambda-bootstrap`" or "your Lambda can't reach the control plane"). No Path B fallback.
- **(b) Keep Path B but warn loudly.** Same poll, but if the agent doesn't show up, fall through to Path B with an operator-visible warning that every step will cold-start a fresh Invoke. Operator decides.

Option (a) is the "no fallback, no workaround, real solution" path the user has asked for repeatedly. **Recommended.**

### 5.2 FaaS: remove Path B from the default exec driver entirely

Once Path A is mandatory, `execStartViaInvoke` becomes dead code for the default flow. It can stay registered as a non-default driver (operator opt-in: "I want exec-via-Invoke for this debug session") but the default `s.Drivers.Exec` slot routes through reverse-agent only. This mirrors the storage-driver pattern where `pd-ephemeral` is registered but not default.

### 5.3 FaaS: drop the per-step `ContainerStart` waiter when the function is already Active

Cycle 1 of `ContainerStart` legitimately needs `FunctionActiveV2Waiter` (function just created, not yet Active). Cycles 2+ on GitLab don't — the function is already Active and we know it because it's in our state. Gate the waiter on `cycle == 1` (track via `LambdaState.StartedCycles` or just `lambdaState.Active`).

### 5.4 FaaS: drop the stdin-pipe poll when there's no attach

The 5-sec poll for `s.stdinPipes.Load(id)` only matters if the runner is sending stdin. For exec-driven runners (GH Actions) and most GitLab stages, there's no stdin attach for cycle 2+. Detect "no /attach happened" and skip the poll.

### 5.5 GCP/Azure FaaS: same Path A/B simplification

`backends/cloudrun-functions/backend_delegates.go:216-223` and `backends/azure-functions/backend_delegates.go:203` have the same structure. Apply the same "mandatory Path A" rule.

### 5.6 Storage: nothing to simplify by default

`gcs-sync` (GCP), `efs-ephemeral` (AWS), `azure-files-ephemeral` (Azure) are each the only realistic default their platform exposes. `pd-ephemeral` is already opt-in.

The user mentioned "stick to a common denominator if we can." There is no common denominator across clouds — each platform has its own persistent-storage primitive. The "common denominator" we *can* enforce is: **one persistent mount per pod, no per-step tar/untar except where the platform forces it (GCP)**. That's already the design.

## 6. Open questions for the user

These need answers before staging a fix-phase:

1. **Path A vs Path B preference for FaaS:** is option 5.1 (a) the right call — fail container start if reverse-agent doesn't connect — or do you want 5.1 (b) (loud warning + fall through)?
2. **Reverse-agent timeout:** how long should we wait for the bootstrap to dial back before failing? Cold-start of an image-based Lambda with VPC + EFS attach is realistically 30–60 sec. Default 60 sec? 90 sec?
3. **GitLab cycle-2 waiter skip (5.3):** safe to assume that if a function transitioned to Active once, it stays Active for the rest of the container's lifetime in the sim? (Real Lambda can return to `Pending` only on config update, which sockerless doesn't trigger mid-job.)
4. **`pd-ephemeral` deprecation level:** keep it in the registry indefinitely as "operator may opt in," or move it to a `legacy/` namespace?
5. **Reverse-agent dial-back transport:** confirm that the control plane is always reachable from inside FaaS workloads in your deployment topology, OR document the "callback URL must be reachable" prerequisite more visibly than the current README.

## 7. What this analysis is NOT

- Not a code change. No fixes yet — the user explicitly asked for analysis first.
- Not a per-bug filing. If we go to a fix phase, BUGs get filed before the first fix commit (standing rule).
- Not a re-design of the driver model. The 4-dimension driver split (network / dns / access / storage) is correct as-is; the simplification opportunities are inside dimensions (FaaS exec driver), not across them.
