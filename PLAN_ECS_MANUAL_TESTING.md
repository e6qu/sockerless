# ECS Manual Testing Plan

Comprehensive manual testing of Sockerless against real AWS ECS Fargate using both `docker` CLI and `podman` CLI, including multi-container pod operations.

## Test Results

### Round 1 (2026-03-29) — Docker CLI basics

11/12 phases pass. Exec skipped. Found and fixed 28 bugs (BUG-584 through BUG-612).

### Round 2 (2026-03-30) — Docker CLI full + Podman CLI + Pods

Found 6 new bugs (BUG-613 through BUG-618). Fixed BUG-618 partially (ping + version prefix).

| Track | Phase | Result | Notes |
|-------|-------|--------|-------|
| A | Docker CLI system/images (A1-A9) | PASS | Real config digest, real history, real sizes |
| A | Docker CLI containers (A10-A16) | PASS | Lifecycle works; ENI IP stays synthetic (BUG-613) |
| A | Docker CLI restart/rename/stats (A17-A25) | PASS (stats=0) | Restart works, rename works, stats zeros (BUG-614) |
| A | Docker CLI memory limit (A29-A30) | FAIL | Invalid Fargate combo produced (BUG-615/616) |
| A | Docker CLI networking (A31-A36) | FAIL | SG not created in AWS (BUG-617) |
| A | Docker CLI volumes (A37) | PASS | In-memory CRUD works |
| A | Docker CLI wait/kill (A43-A44) | PASS | Wait returns 0, kill SIGTERM exits 143 |
| A | Docker CLI system df (A41) | PASS | Disk usage reported |
| A | Docker CLI error cases (A46-A47) | PASS | NotImplemented, 404 correct |
| B | Podman CLI | BLOCKED | Requires full Libpod API (BUG-618) |
| B | Podman pods | BLOCKED | Blocked on BUG-618 |
| C | ECR/Recovery/Advanced | NOT TESTED | Ran out of time |

### Bugs Found

| Bug | Severity | Summary |
|-----|----------|---------|
| BUG-613 | Medium | Real ENI IP never written to container NetworkSettings (pollTaskExit doesn't update RUNNING) |
| BUG-614 | Medium | Container stats return zeros (CloudWatch Container Insights not returning data) |
| BUG-615 | High | fargateResources produces invalid memory values (1536 not valid for 256 CPU) |
| BUG-616 | Medium | -m byte value maps to wrong Fargate tier |
| BUG-617 | High | docker network create fails to create VPC security group on real AWS |
| BUG-618 | High | Podman CLI requires full Libpod API — only pod routes + ping exist |

---

## Infrastructure

Terraform via `terraform/environments/ecs/live/` (eu-west-1).

```bash
cd terraform/environments/ecs/live
source aws.sh && terragrunt apply -auto-approve
```

## Setup

```bash
# Build backend
cd backends/ecs && go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs

# Start backend (export env vars from terragrunt output first)
./sockerless-backend-ecs -addr :9100

# Docker CLI
export DOCKER_HOST=tcp://localhost:9100

# Podman CLI
podman system connection add sockerless-test tcp://localhost:9100
# Then: podman --connection=sockerless-test <command>
```

---

## Track A: Docker CLI

| # | Test | Command | Expected |
|---|------|---------|----------|
| A1 | System info | `docker info` | Shows Driver=ecs-fargate, OS=AWS Fargate |
| A2 | Version | `docker version` | API 1.44, real kernel version from uname |
| A3 | Pull alpine | `docker pull alpine:latest` | Real Cmd=[/bin/sh] from registry |
| A4 | Pull nginx | `docker pull nginx:alpine` | Real Cmd/Entrypoint, real size/layers |
| A5 | Pull python | `docker pull python:3-alpine` | Cmd=[python3] |
| A6 | Inspect image | `docker inspect nginx:alpine` | Real config digest as ID, real RepoDigests |
| A7 | Image history | `docker history nginx:alpine` | Real build steps from OCI config |
| A8 | Tag image | `docker tag alpine myalpine:v1` | Succeeds |
| A9 | Remove tag | `docker rmi myalpine:v1` | Removed |
| A10 | Create container | `docker create --name c1 alpine echo hello` | Returns ID |
| A11 | Start container | `docker start c1` | RunTask, task RUNNING |
| A12 | Inspect running | `docker inspect c1` | Running=true, real ENI IP, derived MAC |
| A13 | List containers | `docker ps` | Shows c1 Up |
| A14 | Logs | `docker logs c1` | "hello" from CloudWatch |
| A15 | Wait for exit | poll `docker ps -a` | Exited(0) |
| A16 | Remove | `docker rm c1` | Task def deregistered |
| A17 | Run detached nginx | `docker run -d --name nginx1 nginx:alpine` | nginx stays running |
| A18 | Still running 60s | `sleep 60 && docker ps` | nginx1 Up |
| A19 | Stats | `docker stats --no-stream nginx1` | Non-zero CPU/memory |
| A20 | Restart | `docker restart nginx1` | New task, fresh TaskDefARN |
| A21 | Verify restart | `docker ps` | Still running, RestartCount=1 |
| A22 | Rename | `docker rename nginx1 web1` | Name updated |
| A23 | Verify rename | `docker inspect web1` | Name=/web1 |
| A24 | Stop | `docker stop web1` | StopTask |
| A25 | Force remove | `docker rm -f web1` | Cleanup |
| A26 | Env vars | `docker run --name env1 -e FOO=bar alpine env` | FOO=bar in logs |
| A27 | Working dir | `docker run --name wd1 -w /tmp alpine pwd` | /tmp in logs |
| A28 | Custom entrypoint | `docker run --name ep1 --entrypoint /bin/echo alpine hi` | "hi" in logs |
| A29 | Memory limit | `docker run -d --name mem1 -m 1g nginx:alpine` | Valid Fargate tier |
| A30 | Verify resources | `aws ecs describe-task-definition ...` | CPU/Memory correct |
| A31 | Network create | `docker network create testnet` | VPC SG created |
| A32 | Verify SG | `aws ec2 describe-security-groups --filters Name=group-name,Values=skls-testnet` | Exists |
| A33 | Run on network | `docker run -d --name net1 --network testnet nginx:alpine` | Task has network SG |
| A34 | Network inspect | `docker network inspect testnet` | Shows net1 |
| A35 | Disconnect/connect | `docker network disconnect testnet net1 && docker network connect testnet net1` | SG updates |
| A36 | Network remove | `docker rm -f net1 && docker network rm testnet` | SG deleted |
| A37 | Volume CRUD | `docker volume create v1 && docker volume inspect v1 && docker volume rm v1` | Lifecycle |
| A38 | Container prune | `docker container prune -f` | Stopped removed |
| A39 | Image prune | `docker image prune -f` | Dangling removed |
| A40 | System prune | `docker system prune -f` | All unused pruned |
| A41 | System df | `docker system df` | Usage reported |
| A42 | Events | `docker events --since 1m &` | Receives events |
| A43 | Container wait | `docker run -d --name w1 alpine sleep 5 && docker wait w1` | Returns 0 |
| A44 | Kill with signal | `docker run -d --name k1 nginx:alpine && docker kill -s SIGTERM k1` | Exits |
| A45 | Double remove | `docker rm k1 && docker rm k1` | Second returns 404 |
| A46 | Pause (error) | `docker pause <running>` | NotImplemented |
| A47 | Inspect nonexistent | `docker inspect nonexistent` | 404 |
| A48 | AWS verification | `aws ecs list-tasks / describe-tasks` | Real resources |
| A49 | CloudWatch verification | `aws logs get-log-events ...` | Real log data |

---

## Track B: Podman CLI

| # | Test | Command | Expected |
|---|------|---------|----------|
| B1 | System info | `podman info` | Backend responds |
| B2 | Version | `podman version` | API version |
| B3 | Pull alpine | `podman pull alpine:latest` | Image pulled |
| B4 | Pull nginx | `podman pull nginx:alpine` | Real config |
| B5 | Create container | `podman create --name pc1 alpine echo hello` | Created |
| B6 | Start container | `podman start pc1` | RunTask on Fargate |
| B7 | Logs | `podman logs pc1` | CloudWatch output |
| B8 | List containers | `podman ps -a` | Shows container |
| B9 | Remove | `podman rm pc1` | Cleanup |
| B10 | Run detached | `podman run -d --name pn1 nginx:alpine` | Running |
| B11 | Stop | `podman stop pn1` | StopTask |
| B12 | Remove | `podman rm pn1` | Cleanup |
| B13 | **Pod create** | `podman pod create --name mypod` | Pod registered |
| B14 | **Pod list** | `podman pod ls` | Shows mypod |
| B15 | **Pod inspect** | `podman pod inspect mypod` | Pod details |
| B16 | **Pod exists** | `podman pod exists mypod` | Exit code 0 |
| B17 | **Add svc1 to pod** | `podman create --pod mypod --name svc1 nginx:alpine` | Associated |
| B18 | **Add svc2 to pod** | `podman create --pod mypod --name svc2 alpine sleep 3600` | Associated |
| B19 | **Pod inspect** | `podman pod inspect mypod` | Shows svc1 + svc2 |
| B20 | **Start svc1** | `podman start svc1` | Deferred (not all started) |
| B21 | **Start svc2** | `podman start svc2` | Triggers multi-container ECS task |
| B22 | **Single ECS task** | `aws ecs list-tasks --cluster sockerless-live` | 1 task for both |
| B23 | **Task definition** | `aws ecs describe-task-definition ...` | 2 container defs |
| B24 | **Both running** | `podman ps` | svc1 + svc2 Up |
| B25 | **Logs from each** | `podman logs svc1 && podman logs svc2` | Output from each |
| B26 | **Pod stop** | `podman pod stop mypod` | Both stopped |
| B27 | **Pod start** | `podman pod start mypod` | New ECS task |
| B28 | **Pod kill** | `podman pod kill mypod` | Both killed |
| B29 | **Pod remove** | `podman pod rm mypod` | Cleaned up |
| B30 | **3-container pod** | Create pod with nginx + redis + alpine | 3-container task |
| B31 | **Verify 3 defs** | `aws ecs describe-task-definition ...` | main + 2 sidecars |
| B32 | **Pod with memory** | `podman create --pod bigpod -m 2g --name big1 nginx:alpine` | Fargate tier >= 2GB |
| B33 | **Network ops** | `podman network create/ls/rm` | VPC SGs |

---

## Track C: Advanced

| # | Test | Command | Expected |
|---|------|---------|----------|
| C1 | ECR login | `aws ecr get-login-password \| docker login --username AWS --password-stdin <ecr>` | Succeeds |
| C2 | Tag for ECR | `docker tag alpine <ecr>:test` | Tagged |
| C3 | Push to ECR | `docker push <ecr>:test` | OCI push |
| C4 | Pull from ECR | `docker pull <ecr>:test` | ECR auth |
| C5 | Run from ECR | `docker run --name ecr1 <ecr>:test echo hi` | Fargate pulls ECR |
| C6 | Image load | `docker save alpine \| docker load` | Layers preserved |
| C7 | Recovery | Kill backend, restart, `docker ps` | Containers recovered |
| C8 | Container diff | `docker diff <id>` | NotImplemented |
| C9 | Container export | `docker export <id>` | NotImplemented |

---

## Teardown

```bash
cd terraform/environments/ecs/live && source aws.sh
terragrunt destroy -auto-approve
aws s3 rb s3://sockerless-tf-state --region eu-west-1 --force
aws ecs list-task-definitions --region eu-west-1 --query 'taskDefinitionArns[?contains(@,`sockerless`)]'
```

---

## Known Limitations

- No container pause/unpause (Fargate limitation)
- CloudWatch log latency (2-10s before logs appear)
- Exec requires agent or SSM (not tested without agent setup)
- CPU/memory rounded to nearest valid Fargate tier
- Bind mounts are EFS-backed when AgentEFSID set, otherwise scratch
- NAT Gateway cost ~$0.045/hr while infrastructure exists
