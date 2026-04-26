# AWS runbook — ECS Fargate + Lambda

Walk every track end-to-end against live AWS. Provision per [01-infrastructure.md](01-infrastructure.md), run, tear down. Each row is a single test; record pass / fail / NotImpl per round.

## Track A — Docker CLI, core lifecycle (ECS + Lambda)

| # | Test | Command | Expected |
|---|---|---|---|
| A1 | System info | `docker info` | `Driver=ecs-fargate` (or `lambda`) |
| A2 | Version | `docker version` | API 1.44 |
| A3–A9 | Image pull / inspect / history / tag / rmi | `docker pull alpine:latest`, `docker inspect alpine`, `docker history alpine`, `docker tag / rmi` | Real registry digests; history has a real entry. |
| A10–A16 | Container lifecycle | `docker create / start / inspect / ps / logs / wait / rm` | Real Fargate/Lambda task; logs from CloudWatch; `docker wait` returns real exit code. |
| A17–A25 | Detached nginx (ECS only) | `docker run -d / ps / stats / restart / rename / stop / rm -f` | Real task lifecycle. Stats = zeros when CloudWatch has no data (no fake PIDs). |
| A26–A28 | Env / workdir / entrypoint | `docker run -e FOO=bar / -w /tmp / --entrypoint echo` | Overrides propagate through to the task/function env. |
| A29–A30 | Resources | `docker run -d -m 1g nginx:alpine` + `aws ecs describe-task-definition` | CPU/Mem rounded to a valid Fargate tier (ECS) or mapped to Lambda `MemorySize`. |
| A31–A36 | Network CRUD (ECS) | `docker network create/inspect/rm testnet` + per-container attach/detach | Real VPC SG created; container ENIs tagged; SG cleanup on `network rm`. |
| A37 | Named volumes | `docker volume create v1 && docker run -v v1:/data alpine touch /data/x && docker run -v v1:/data alpine ls /data` | File persists across runs (EFS). |
| A38–A42 | Prune + events | `docker container/image/system prune -f`, `docker events --since 1m` | Real prune; events emitted. |
| A43 | Wait | `docker run -d alpine sleep 5 && docker wait <id>` | Returns `0`. |
| A44 | Kill with signal | `docker run -d nginx:alpine && docker kill -s SIGTERM <id>` | Exits `143` (128+15). |
| A45 | Double remove | `docker rm <gone> && docker rm <gone>` | First succeeds; second returns 404. |
| A46 | Pause / unpause via reverse-agent | `docker run -d --name p1 alpine sleep 3600 && docker pause p1 && docker unpause p1` | Pause succeeds on Lambda when the bootstrap writes `/tmp/.sockerless-mainpid`. ECS uses SSM ExecuteCommand for the same path. |
| A47 | Inspect nonexistent | `docker inspect nonexistent` | 404. |
| A48 | AWS verification | `aws ecs list-tasks && aws ecs describe-tasks` | Task ARNs present. |
| A49 | CloudWatch logs | `aws logs get-log-events ...` | Real log stream; subprocess stdout reaches CloudWatch (not just START/END/REPORT). |

**Lambda-specific:** A17–A25 (detached nginx) not applicable — Lambda is invocation-scoped. A29–A30 use Lambda memory tiers. A31–A36 networking not applicable.

## Track B — Podman CLI

| # | Test | Command | Expected |
|---|---|---|---|
| B1–B2 | System info / version | `podman info / version` | Backend responds. |
| B3–B4 | Pull | `podman pull alpine:latest / nginx:alpine` | libpod format. |
| B5–B12 | Container lifecycle | `podman create / start / logs / ps -a / rm / run -d / stop` | Same semantics as docker track. |
| B13–B16 | Pod create / list / inspect / exists | `podman pod create --name mypod && podman pod ls / inspect / exists` | Pod registered in Store.Pods + `sockerless-pod=<name>` label on cloud resource. |
| B29 | Pod remove | `podman pod rm mypod` | `PodRmReport` shape. |
| B33 | Network ops | `podman network ls` | Bridge/host/none surfaced. |

## Track C — Advanced (registry + agent-driven ops)

| # | Test | Command | Expected |
|---|---|---|---|
| C1 | ECR login | `aws ecr get-login-password \| docker login ...` | Succeeds. |
| C2 | Tag for ECR | `docker tag alpine <ecr-host>/alpine:test` | Tagged. |
| C3 | Push to ECR | `docker push <ecr-host>/alpine:test` | Real OCI push — layers + config + manifest. `aws ecr describe-images` shows the tag. |
| C4 | Diff | `docker diff <running-cid>` | Lists files added/modified since container boot. (Deletions not captured by the find-based path; documented limitation. Overlay-rootfs mode opt-in via `SOCKERLESS_OVERLAY_ROOTFS=1` captures whiteouts.) |
| C5 | Export | `docker export <cid> > out.tar && tar tvf out.tar` | Full rootfs tarball via reverse-agent. |
| C6 | Stat | `docker container stat <cid>:/etc/hostname` | Returns `ContainerPathStat` with real mode/size. |
| C7 | Cp host→container | `docker cp ./foo.txt <cid>:/tmp/foo.txt && docker exec <cid> cat /tmp/foo.txt` | File extracted via `tar -xf -`. |
| C8 | Cp container→host | `docker cp <cid>:/etc/hostname ./host.txt && cat host.txt` | Tarball streamed back. |
| C9 | Top | `docker top <cid> aux` | Real `ps` output via reverse-agent. |
| C10 | Commit (opt-in) | `SOCKERLESS_ENABLE_COMMIT=1` + `docker commit <cid> myimage:snap` | New image ID; `docker inspect myimage:snap` shows merged config + parent image. `docker push myimage:snap` to ECR works. |
| C11 | Image-history after pull | `docker history nginx:alpine` | Real build-step entries from registry-sourced history (no synthetic layer text). |
| C12 | Restart-count tag | `docker run -d --name r-c12 alpine sleep 600 && docker restart r-c12 && docker restart r-c12 && docker inspect r-c12 --format '{{.RestartCount}}'` + ECS task tag check | `RestartCount=2`; ECS task tag `sockerless-restart-count=2`. |
| C13 | Kill-signal tag + exit code | `docker run -d --name r-c13 nginx:alpine && docker kill -s SIGTERM r-c13 && docker wait r-c13` + ECS task tag check | ExitCode = 128 + SignalNumber (e.g. 143 for SIGTERM); tag `sockerless-kill-signal=SIGTERM`. |
| C14 | ImagePush layer-byte content (Docker Hub source) | `docker pull docker.io/library/busybox:1.36 && docker tag … && docker push <ecr>/busybox:test` + manifest digest check | Manifest digests match the source image's compressed-layer digests verbatim — no recompute. |
| C15 | LayerContent cache eviction | `docker rmi <image-with-large-layers> && docker pull <image-with-large-layers>` | Second pull re-fetches blobs (LayerContent is gone after rmi). |

## Track D — Lambda-specific

| # | Test | Command | Expected |
|---|---|---|---|
| D1 | Info | `docker info` (on :9200) | Driver=lambda. |
| D2–D3 | Create + start | `docker create --name l1 alpine echo hello && docker start l1` | Real Lambda invocation. |
| D4 | Logs | `docker logs l1` after the invoke completes | Sees `hello`. START/END/REPORT also present from the runtime. |
| D5 | Exit code | `docker inspect l1 --format '{{.State.ExitCode}}'` | 0 on success, 1 on error. |
| D6 | Error propagation | `docker run --name l2 alpine false` | ExitCode=1, State.Error set. |
| D7 | Env vars | `docker run -e KEY=val alpine env` | KEY=val in output. |
| D8 | Exec via reverse-agent | `docker exec l1 ps aux` | Real process list (requires `SOCKERLESS_CALLBACK_URL` + overlay image with `sockerless-lambda-bootstrap`). |
| D9 | Attach via reverse-agent | `docker attach -i l1` | Interactive bidi (with agent) or log-streamed attach (no agent — `core.AttachViaCloudLogs`). |

## Track E — Container-to-container (ECS)

| # | Test | Command | Expected |
|---|---|---|---|
| E1 | Create network | `docker network create commsnet` | VPC SG created. |
| E2–E3 | Server + IP | `docker run -d --name web --network commsnet nginx:alpine && docker inspect web` | Real ENI IP (10.x). |
| E4 | Peer curl | `docker run --rm --network commsnet alpine wget -qO- http://<web-ip>` | nginx HTML. |
| E5 | Shared SG | `aws ecs describe-tasks ...` | Both tasks carry `skls-commsnet`. |
| E6 | Cross-network isolation | `docker run --rm alpine wget -qO- --timeout=5 http://<web-ip>` | Timeout. |
| E7 | Cleanup | `docker rm -f web && docker network rm commsnet` | SG deleted. |

## Track F — Podman pods on ECS

| # | Test | Command | Expected |
|---|---|---|---|
| F1 | Create pod | `podman pod create --name mypod` | Pod registered. |
| F2 | Add nginx | `podman create --pod mypod --name svc1 nginx:alpine` | Associated. |
| F3 | Add sidecar | `podman create --pod mypod --name svc2 alpine sleep 3600` | Associated. |
| F4 | Pod inspect | `podman pod inspect mypod` | Shows svc1 + svc2. |
| F5 | Start svc1 | `podman start svc1` | Deferred (not all started). |
| F6 | Start svc2 | `podman start svc2` | Single multi-container ECS task. |
| F7 | Task listing | `aws ecs list-tasks` | 1 task. |
| F8 | Task def | `aws ecs describe-task-definition` | 2 container definitions. |
| F9 | Both running | `podman ps` | svc1 + svc2 Up. |
| F10 | Localhost comms | inside svc2: `wget -qO- http://localhost:80` | nginx response (shared netns). |
| F11–F12 | Stop + rm | `podman pod stop mypod && podman pod rm mypod` | Cleaned up. |

## Track G — Docker Compose

```yaml
# /tmp/compose-test/docker-compose.yml
services:
  web:
    image: nginx:alpine
    ports: ["8080:80"]
  worker:
    image: alpine
    command: ["sleep", "3600"]
    depends_on: [web]
```

| # | Test | Expected |
|---|---|---|
| G1 | `docker compose up -d` | Both services created + started. |
| G2 | `docker compose ps` | Both running. |
| G3 | `docker compose logs web` | nginx access logs. |
| G4 | `docker compose exec worker echo hello` | "hello" (requires reverse-agent). |
| G5 | `docker compose stop` | Both stopped. |
| G6 | `docker compose down` | Containers + network removed. |
| G7 | `docker compose down -v` | Volumes removed too. |

## Track H — Podman Compose

```yaml
# /tmp/podman-compose-test/docker-compose.yml
services:
  api:
    image: nginx:alpine
  cache:
    image: alpine
    command: ["sleep", "3600"]
```

| # | Test | Expected |
|---|---|---|
| H1 | `podman-compose up -d` | Both created. |
| H2 | `podman-compose ps` | Both running. |
| H3 | `podman-compose down` | Cleaned up. |

## Track I — Stateless backend verification

| # | Test | Expected |
|---|---|---|
| I1 | `docker run -d --name persist1 nginx:alpine` | Running on Fargate. |
| I2 | `docker ps` | Shows persist1. |
| I3 | `pkill sockerless-backend-ecs` | Backend exits. |
| I4 | Restart backend with same config | Starts fresh; zero local state. |
| I5 | `docker ps` | Shows persist1 (derived from ECS cloud state). |
| I6 | `docker inspect persist1` | Full details from ECS. |
| I7 | `docker stop persist1` | StopTask works using cloud-resolved ARN. |
| I8 | `docker ps -a` | Shows Exited. |
| I9 | `docker rm persist1` | Cleaned. |

## Track J — Runner integration

Use sockerless as the docker daemon for GitLab Runner / GitHub Actions self-hosted runner.

| # | Test | Expected |
|---|---|---|
| J1 | GitLab Runner docker executor against CR Services (`SOCKERLESS_GCR_USE_SERVICE=1`) | Job runs; `docker exec -i` for each step works via the bootstrap. |
| J2 | GitHub Actions container-job against ACA Apps (`SOCKERLESS_ACA_USE_APP=1`) | `tail -f /dev/null` keep-alive + `docker exec` per step. |
| J3 | Same against ECS | Works with SSM ExecuteCommand. |
| J4 | Same against Lambda / CR Jobs / ACA Jobs / GCF / AZF | **Expected to fail** — invocation-scoped, no keep-alive. Spec says NotSupported for these. |

Matrix + GitLab/GitHub runner design analysis in [specs/CLOUD_RESOURCE_MAPPING.md](../specs/CLOUD_RESOURCE_MAPPING.md) §Exec.

## Known limitations

- **CloudWatch log latency** — 2–10 s before logs appear. `docker logs -f` tolerates this.
- **Fargate CPU/Memory tiers** — rounded to the nearest valid combo.
- **Lambda 15-minute max** — no long-running Lambda containers.
- **ECS reverse-agent ops via SSM** — export / top / diff / stat / cp / pause depend on a task-def convention that writes `/tmp/.sockerless-mainpid`. Without that, pause/unpause returns NotImplementedError.
