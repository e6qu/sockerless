# Sockerless — Manual Testing Plan

Manual testing of Sockerless against real AWS ECS Fargate + Lambda, Cloud Run Jobs/Services, ACA Jobs/Apps, Cloud Functions, and Azure Functions, using `docker` CLI, `podman` CLI, and multi-container pods.

Rolling run history is in `docs/manual-test-history.md`. This file is the current runbook.

## Results summary (most recent rounds)

| Round | Date | Backend | Score |
|---|---|---|---|
| R7 | TBD post-PR-#115 | ECS live | Needs re-run to exercise Phase 98/98b/99/101/102 paths. |
| R6 | 2026-04-05 | ECS live | Docker CLI all pass; Podman pull + pods pass (container ops blocked by response format); Advanced 3/4. |
| R5 | 2026-04-05 | ECS live | 43/46; 4 bugs found+fixed. |
| R1–R4 | 2026-03-29 → 04-05 | ECS live | 28 bugs fixed across rounds. |

Pre-PR-#115 status: A46 / C8 / C9 (pause / diff / export) were all `NotImplemented`. They are **implemented now** — any re-run must exercise them.

## Infrastructure

Shared VPC via `terraform/environments/ecs/live/` (eu-west-1). Lambda reuses the same subnets + security group.

```bash
# ECS infrastructure: VPC, subnets, SGs, cluster, EFS, ECR, IAM, Cloud Map.
cd terraform/environments/ecs/live
source aws.sh && terragrunt apply -auto-approve
terragrunt output -json | python3 -c "
import json, sys
d = json.load(sys.stdin)
for k,v in {
  'AWS_REGION': d['region']['value'],
  'SOCKERLESS_ECS_CLUSTER': d['ecs_cluster_name']['value'],
  'SOCKERLESS_ECS_SUBNETS': ','.join(d['private_subnet_ids']['value']),
  'SOCKERLESS_ECS_SECURITY_GROUPS': d['task_security_group_id']['value'],
  'SOCKERLESS_ECS_TASK_ROLE_ARN': d['task_role_arn']['value'],
  'SOCKERLESS_ECS_EXECUTION_ROLE_ARN': d['execution_role_arn']['value'],
  'SOCKERLESS_ECS_LOG_GROUP': d['log_group_name']['value'],
  'SOCKERLESS_AGENT_EFS_ID': d['efs_filesystem_id']['value'],
  'SOCKERLESS_ECS_PUBLIC_IP': 'true',
}.items(): print(f'export {k}={v}')
" > /tmp/ecs-env.sh

# Lambda infrastructure: IAM role, log group, ECR repo.
cd terraform/environments/lambda/live
source aws.sh && terragrunt apply -auto-approve
LAMBDA_ROLE=$(terragrunt output -json | python3 -c "import json,sys;print(json.load(sys.stdin)['execution_role_arn']['value'])")
```

## Setup

```bash
# Build backends.
cd backends/ecs    && go build -o sockerless-backend-ecs    ./cmd/sockerless-backend-ecs
cd backends/lambda && go build -o sockerless-backend-lambda ./cmd/sockerless-backend-lambda

# ECS backend on :3375.
source aws.sh && source /tmp/ecs-env.sh
./sockerless-backend-ecs -addr :3375 &

# Lambda backend on :9200 (shares VPC from ECS).
export SOCKERLESS_LAMBDA_ROLE_ARN=$LAMBDA_ROLE
export SOCKERLESS_LAMBDA_SUBNETS=$SOCKERLESS_ECS_SUBNETS
export SOCKERLESS_LAMBDA_SECURITY_GROUPS=$SOCKERLESS_ECS_SECURITY_GROUPS
# Enable docker commit on Lambda (Phase 98b):
export SOCKERLESS_ENABLE_COMMIT=1
# Enable docker exec/attach/pause/cp via reverse-agent:
export SOCKERLESS_CALLBACK_URL="wss://<public-endpoint-for-lambda>/v1/lambda/reverse"
./sockerless-backend-lambda -addr :9200 &

# Docker CLI against either backend.
export DOCKER_HOST=tcp://localhost:3375  # ECS
# or: export DOCKER_HOST=tcp://localhost:9200  # Lambda

# Podman CLI.
podman system connection add sockerless-ecs    tcp://localhost:3375
podman system connection add sockerless-lambda tcp://localhost:9200
```

---

## Track A — Docker CLI, core lifecycle (ECS + Lambda)

| # | Test | Command | Expected |
|---|---|---|---|
| A1 | System info | `docker info` | `Driver=ecs-fargate` (or `lambda`) |
| A2 | Version | `docker version` | API 1.44 |
| A3–A9 | Image pull / inspect / history / tag / rmi | `docker pull alpine:latest`, `docker inspect alpine`, `docker history alpine`, `docker tag / rmi` | Real registry digests; history has a real entry (no fake per-layer text — BUG-769). |
| A10–A16 | Container lifecycle | `docker create / start / inspect / ps / logs / wait / rm` | RunTask/Invoke real Fargate/Lambda; logs from CloudWatch; `docker wait` returns real exit code. |
| A17–A25 | Detached nginx (ECS only) | `docker run -d / ps / stats / restart / rename / stop / rm -f` | Real task lifecycle. Stats = zeros when CloudWatch has no data (BUG-733 ensures we don't fake PIDs=1). |
| A26–A28 | Env / workdir / entrypoint | `docker run -e FOO=bar / -w /tmp / --entrypoint echo` | Overrides propagate through to the task/function env. |
| A29–A30 | Resources | `docker run -d -m 1g nginx:alpine` + `aws ecs describe-task-definition` | CPU/Mem rounded to a valid Fargate tier (ECS) or mapped to Lambda `MemorySize`. |
| A31–A36 | Network CRUD (ECS) | `docker network create/inspect/rm testnet` + per-container attach/detach | Real VPC SG created; container ENIs tagged; SG cleanup on `network rm`. |
| A37 | Named volumes | `docker volume create v1 && docker run -v v1:/data alpine touch /data/x && docker run -v v1:/data alpine ls /data` | File persists across runs (EFS on ECS / Lambda, GCS on CR/GCF, Azure Files on ACA/AZF). |
| A38–A42 | Prune + events | `docker container/image/system prune -f`, `docker events --since 1m` | Real prune; events emitted. |
| A43 | Wait | `docker run -d alpine sleep 5 && docker wait <id>` | Returns `0`. |
| A44 | Kill with signal | `docker run -d nginx:alpine && docker kill -s SIGTERM <id>` | Exits `143`. |
| A45 | Double remove | `docker rm <gone> && docker rm <gone>` | First succeeds; second returns 404. |
| A46 | **Pause / unpause (reverse-agent, Phase 99)** | `docker run -d --name p1 alpine sleep 3600 && docker pause p1 && docker unpause p1` | Pause succeeds on Lambda/CR/ACA/GCF/AZF when bootstrap writes `/tmp/.sockerless-mainpid`. ECS returns `NotImplementedError` (Phase 102 plans SSM path). |
| A47 | Inspect nonexistent | `docker inspect nonexistent` | 404. |
| A48 | AWS verification | `aws ecs list-tasks && aws ecs describe-tasks` | Task ARNs present. |
| A49 | CloudWatch logs | `aws logs get-log-events ...` | Real log stream; subprocess stdout reaches CloudWatch (BUG-756 — no longer only START/END/REPORT). |

**Lambda-specific:** A17–A25 (detached nginx) not applicable — Lambda is invocation-scoped. A29–A30 use Lambda memory tiers. A31–A36 networking not applicable.

---

## Track B — Podman CLI

| # | Test | Command | Expected |
|---|---|---|---|
| B1–B2 | System info / version | `podman info / version` | Backend responds. |
| B3–B4 | Pull | `podman pull alpine:latest / nginx:alpine` | libpod format. |
| B5–B12 | Container lifecycle | `podman create / start / logs / ps -a / rm / run -d / stop` | Same semantics as docker track. |
| B13–B16 | Pod create / list / inspect / exists | `podman pod create --name mypod && podman pod ls / inspect / exists` | Pod registered in Store.Pods + `sockerless-pod=<name>` label on cloud resource. |
| B29 | Pod remove | `podman pod rm mypod` | `PodRmReport` shape. |
| B33 | Network ops | `podman network ls` | Bridge/host/none surfaced. |

---

## Track C — Advanced (registry + agent-driven ops)

| # | Test | Command | Expected |
|---|---|---|---|
| C1 | ECR login | `aws ecr get-login-password \| docker login ...` | Succeeds. |
| C2 | Tag for ECR | `docker tag alpine <ecr-host>/alpine:test` | Tagged. |
| C3 | Push to ECR | `docker push <ecr-host>/alpine:test` | Real OCI push — layers + config + manifest (BUG-763/764). `aws ecr describe-images` shows the tag. |
| C4 | **Diff (Phase 98)** | `docker diff <running-cid>` | Lists files added/modified since container boot. (Deletions not captured — find-based; documented limitation.) |
| C5 | **Export (Phase 98)** | `docker export <cid> > out.tar && tar tvf out.tar` | Full rootfs tarball via reverse-agent. |
| C6 | **Stat (Phase 98)** | `docker container stat <cid>:/etc/hostname` | Returns `ContainerPathStat` with real mode/size. |
| C7 | **Cp host→container (Phase 98)** | `docker cp ./foo.txt <cid>:/tmp/foo.txt && docker exec <cid> cat /tmp/foo.txt` | File extracted via `tar -xf -`. |
| C8 | **Cp container→host (Phase 98)** | `docker cp <cid>:/etc/hostname ./host.txt && cat host.txt` | Tarball streamed back. |
| C9 | **Top (Phase 98)** | `docker top <cid> aux` | Real `ps` output via reverse-agent. |
| C10 | **Commit (Phase 98b, opt-in)** | `SOCKERLESS_ENABLE_COMMIT=1` + `docker commit <cid> myimage:snap` | New image ID; `docker inspect myimage:snap` shows merged config + parent image. `docker push myimage:snap` to ECR works. |
| C11 | Image-history after pull | `docker history nginx:alpine` | Real build-step entries (via registry-sourced history — BUG-769 means no more fake `ADD file:... in /` entries). |
| C12 | Restart-count tag | `docker run -d --name r9-c12 alpine sleep 600 && docker restart r9-c12 && docker restart r9-c12 && docker inspect r9-c12 --format '{{.RestartCount}}' && aws ecs describe-tasks --cluster $C --tasks $(docker inspect r9-c12 --format '{{.ID}}') --query 'tasks[].tags[?key==\`sockerless-restart-count\`].value' --output text` | `RestartCount=2`; ECS task tag `sockerless-restart-count=2`. (BUG-772 round-7.) |
| C13 | Kill-signal tag + exit code | `docker run -d --name r9-c13 nginx:alpine && docker kill -s SIGTERM r9-c13 && docker wait r9-c13 && docker inspect r9-c13 --format '{{.State.ExitCode}}' && aws ecs describe-tasks --cluster $C --tasks <arn> --query 'tasks[].tags[?key==\`sockerless-kill-signal\`].value' --output text` | ExitCode = 128 + SignalNumber (e.g. 143 for SIGTERM); tag `sockerless-kill-signal=SIGTERM`. (BUG-781.) |
| C14 | ImagePush layer-byte content (non-public source) | `docker pull docker.io/library/busybox:1.36 && docker tag docker.io/library/busybox:1.36 <ecr>/busybox:test && docker push <ecr>/busybox:test && aws ecr batch-get-image --repository-name <repo> --image-ids imageTag=test --accepted-media-types application/vnd.oci.image.manifest.v1+json --query 'images[0].imageManifest' --output text \| jq '.layers[].digest'` | Manifest digests match the source image's compressed-layer digests verbatim — no recompute. (BUG-788; verifies registry-to-registry mirror works for Docker Hub sources, not just public.ecr.aws.) |
| C15 | LayerContent cache eviction | `docker rmi <image-with-large-layers> && docker images && docker pull <image-with-large-layers> && docker push <ecr>/<image>:test` | After `rmi`, `Store.LayerContent` for the removed image's layer digests is gone (verifiable via memory snapshot or by running the test twice and confirming the second pull re-fetches blobs). (BUG-788 follow-up — confirms the fix doesn't permanently leak layer bytes.) |

---

## Track D — Lambda-specific

| # | Test | Command | Expected |
|---|---|---|---|
| D1 | Info | `docker info` (on :9200) | Driver=lambda. |
| D2–D3 | Create + start | `docker create --name l1 alpine echo hello && docker start l1` | Real Lambda invocation. |
| D4 | Logs (follow-mode includes subprocess stdout) | `docker logs l1` after the invoke completes | Sees `hello` (BUG-756). START/END/REPORT also present from the runtime. |
| D5 | Exit code | `docker inspect l1 --format '{{.State.ExitCode}}'` | 0 on success, 1 on error (BUG-744 invocation-lifecycle tracker). |
| D6 | Error propagation | `docker run --name l2 alpine false` | ExitCode=1, State.Error set. |
| D7 | Env vars | `docker run -e KEY=val alpine env` | KEY=val in output. |
| D8 | Exec via reverse-agent | `docker exec l1 ps aux` | Real process list (requires `SOCKERLESS_CALLBACK_URL` + overlay image with `sockerless-lambda-bootstrap`). |
| D9 | Attach via reverse-agent | `docker attach -i l1` | Interactive bidi (with agent) or log-streamed attach (no agent — `core.AttachViaCloudLogs`). |

---

## Track E — Container-to-container (ECS)

| # | Test | Command | Expected |
|---|---|---|---|
| E1 | Create network | `docker network create commsnet` | VPC SG created. |
| E2–E3 | Server + IP | `docker run -d --name web --network commsnet nginx:alpine && docker inspect web` | Real ENI IP (10.x). |
| E4 | Peer curl | `docker run --rm --network commsnet alpine wget -qO- http://<web-ip>` | nginx HTML. |
| E5 | Shared SG | `aws ecs describe-tasks ...` | Both tasks carry `skls-commsnet`. |
| E6 | Cross-network isolation | `docker run --rm alpine wget -qO- --timeout=5 http://<web-ip>` | Timeout. |
| E7 | Cleanup | `docker rm -f web && docker network rm commsnet` | SG deleted. |

---

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

---

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

---

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

---

## Track J — Runner integration (Phase 101 doc)

Use sockerless as the docker daemon for GitLab Runner / GitHub Actions self-hosted runner.

| # | Test | Expected |
|---|---|---|
| J1 | GitLab Runner docker executor against CR Services (`SOCKERLESS_GCR_USE_SERVICE=1`) | Job runs; `docker exec -i` for each step works via the bootstrap. |
| J2 | GitHub Actions container-job against ACA Apps (`SOCKERLESS_ACA_USE_APP=1`) | `tail -f /dev/null` keep-alive + `docker exec` per step. |
| J3 | Same against ECS | Works with SSM ExecuteCommand. |
| J4 | Same against Lambda / CR Jobs / ACA Jobs / GCF / AZF | **Expected to fail** — invocation-scoped, no keep-alive. Spec says NotSupported for these. |

Matrix + GitLab/GitHub runner design analysis in `specs/CLOUD_RESOURCE_MAPPING.md` §Exec.

---

## Teardown

```bash
source aws.sh

# Stop running tasks first (takes ~60s).
for task in $(aws ecs list-tasks --cluster sockerless-live --region eu-west-1 --query 'taskArns[]' --output text); do
  aws ecs stop-task --cluster sockerless-live --task "$task" --region eu-west-1
done
sleep 60

# Destroy.
cd terraform/environments/ecs/live    && terragrunt destroy -auto-approve
cd terraform/environments/lambda/live && terragrunt destroy -auto-approve

# Deregister orphan task definitions.
for td in $(aws ecs list-task-definitions --region eu-west-1 --status ACTIVE \
  --query 'taskDefinitionArns[?contains(@,`sockerless`)]' --output text); do
  aws ecs deregister-task-definition --task-definition "$td" --region eu-west-1
done

# Optional: remove tf state bucket.
aws s3 rb s3://sockerless-tf-state --region eu-west-1 --force
```

## Known limitations

- **CloudWatch log latency** — 2–10 s before logs appear. `docker logs -f` tolerates this.
- **Fargate CPU/Memory tiers** — rounded to the nearest valid combo.
- **Lambda 15-minute max** — no long-running Lambda containers.
- **ECS reverse-agent ops (Phase 102)** — export / top / diff / stat / cp / pause via SSM ExecuteCommand are wired but depend on a task-def convention that writes `/tmp/.sockerless-mainpid`. Without that, pause/unpause returns the Phase 102 NotImplementedError message.
- **NAT Gateway cost** ~$0.045/hr while the ECS VPC is up.
