# AWS Manual Testing Plan

Manual testing of Sockerless against real AWS ECS Fargate and Lambda using `docker` CLI, `podman` CLI, and multi-container pods. Both backends share the same VPC infrastructure.

## Results Summary

| Round | Date | ECS | Lambda | Podman | Bugs |
|-------|------|-----|--------|--------|------|
| R1 | 2026-03-29 | 11/12 | — | — | 28 fixed |
| R2 | 2026-03-30 | partial | — | blocked | 6 found |
| R3 | 2026-04-04 | 38/41 | — | 6/33 | 19 fixed |
| R4 | 2026-04-05 | verified | — | 7/8 | 3 found |
| R5 | 2026-04-05 | 43/46 | — | 7/8 | 4 found+fixed |
| R6 | 2026-04-05 | all pass | — | pull works, containers blocked | 1 found+fixed |

## Infrastructure

Shared VPC via `terraform/environments/ecs/live/` (eu-west-1). Lambda uses the same subnets and security group.

```bash
# Provision ECS infrastructure (creates VPC, subnets, SGs, ECS cluster, EFS, ECR, IAM, Cloud Map)
cd terraform/environments/ecs/live
source aws.sh && terragrunt apply -auto-approve

# Extract env vars
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
" | tee /tmp/ecs-env.sh

# Provision Lambda infrastructure (IAM role, log group, ECR repo)
cd terraform/environments/lambda/live
source aws.sh && terragrunt apply -auto-approve
# Extract Lambda role ARN
LAMBDA_ROLE=$(terragrunt output -json 2>/dev/null | python3 -c "import json,sys;print(json.load(sys.stdin)['execution_role_arn']['value'])")
```

## Setup

```bash
# Build both backends
cd backends/ecs && go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs
cd backends/lambda && go build -o sockerless-backend-lambda ./cmd/sockerless-backend-lambda

# Start ECS backend on :9100
source aws.sh && source /tmp/ecs-env.sh
./sockerless-backend-ecs -addr :9100

# Start Lambda backend on :9200 (shares VPC from ECS)
export SOCKERLESS_LAMBDA_ROLE_ARN=$LAMBDA_ROLE
export SOCKERLESS_LAMBDA_SUBNETS=$SOCKERLESS_ECS_SUBNETS
export SOCKERLESS_LAMBDA_SECURITY_GROUPS=$SOCKERLESS_ECS_SECURITY_GROUPS
./sockerless-backend-lambda -addr :9200

# Docker CLI
export DOCKER_HOST=tcp://localhost:9100  # ECS
# or: export DOCKER_HOST=tcp://localhost:9200  # Lambda

# Podman CLI
podman system connection add sockerless-ecs tcp://localhost:9100
podman system connection add sockerless-lambda tcp://localhost:9200
```

---

## Track A: Docker CLI (both backends)

| # | Test | Command | Expected |
|---|------|---------|----------|
| A1 | System info | `docker info` | Driver=ecs-fargate (or lambda) |
| A2 | Version | `docker version` | API 1.44 |
| A3 | Pull alpine | `docker pull alpine:latest` | Real digest |
| A4 | Pull nginx | `docker pull nginx:alpine` | Real config |
| A5 | Pull python | `docker pull python:3-alpine` | Cmd=[python3] |
| A6 | Inspect image | `docker inspect nginx:alpine` | Real config digest as ID |
| A7 | Image history | `docker history nginx:alpine` | Real build steps |
| A8 | Tag image | `docker tag alpine myalpine:v1` | Succeeds |
| A9 | Remove tag | `docker rmi myalpine:v1` | Removed |
| A10 | Create container | `docker create --name c1 alpine echo hello` | Returns ID |
| A11 | Start container | `docker start c1` | RunTask / Invoke |
| A12 | Inspect running | `docker inspect c1` | Running=true |
| A13 | List containers | `docker ps` | Shows c1 |
| A14 | Logs | `docker logs c1` | "hello" from CloudWatch |
| A15 | Wait for exit | poll `docker ps -a` | Exited(0) |
| A16 | Remove | `docker rm c1` | Removed |
| A17 | Run detached nginx | `docker run -d --name nginx1 nginx:alpine` | Stays running |
| A18 | Still running 60s | `sleep 60 && docker ps` | nginx1 Up |
| A19 | Stats | `docker stats --no-stream nginx1` | Zeros (no Insights) |
| A20 | Restart | `docker restart nginx1` | New task |
| A21 | Verify restart | `docker ps` | RestartCount=1 |
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
| A32 | Verify SG | `aws ec2 describe-security-groups ...` | Exists |
| A33 | Run on network | `docker run -d --name net1 --network testnet nginx:alpine` | Task has network SG |
| A34 | Network inspect | `docker network inspect testnet` | Shows net1 |
| A35 | Disconnect/connect | `docker network disconnect/connect testnet net1` | SG updates |
| A36 | Network remove | `docker rm -f net1 && docker network rm testnet` | SG deleted |
| A37 | Volume CRUD | `docker volume create/inspect/rm v1` | Lifecycle |
| A38 | Container prune | `docker container prune -f` | Stopped removed |
| A39 | Image prune | `docker image prune -f` | Dangling removed |
| A40 | System prune | `docker system prune -f` | Works |
| A41 | System df | `docker system df` | Non-negative |
| A42 | Events | `docker events --since 1m &` | Receives events |
| A43 | Container wait | `docker run -d --name w1 alpine sleep 5 && docker wait w1` | Returns 0 |
| A44 | Kill with signal | `docker run -d --name k1 nginx:alpine && docker kill -s SIGTERM k1` | Exits 143 |
| A45 | Double remove | `docker rm k1 && docker rm k1` | Second returns 404 |
| A46 | Pause (error) | `docker pause <running>` | NotImplemented |
| A47 | Inspect nonexistent | `docker inspect nonexistent` | 404 |
| A48 | AWS verification | `aws ecs list-tasks / describe-tasks` | Real resources |
| A49 | CloudWatch logs | `aws logs get-log-events ...` | Real log data |

**Lambda-specific notes:** A17-A25 (detached nginx) not applicable — Lambda is FaaS, containers exit after invocation. A29-A30 use Lambda memory tiers. A31-A36 networking not applicable.

---

## Track B: Podman CLI

| # | Test | Command | Expected |
|---|------|---------|----------|
| B1 | System info | `podman info` | Backend responds |
| B2 | Version | `podman version` | API version |
| B3 | Pull alpine | `podman pull alpine:latest` | Pulled with libpod format |
| B4 | Pull nginx | `podman pull nginx:alpine` | Real config |
| B5 | Create container | `podman create --name pc1 alpine echo hello` | Created |
| B6 | Start container | `podman start pc1` | RunTask/Invoke |
| B7 | Logs | `podman logs pc1` | Output |
| B8 | List containers | `podman ps -a` | Shows container |
| B9 | Remove | `podman rm pc1` | Cleanup |
| B10 | Run detached | `podman run -d --name pn1 nginx:alpine` | Running |
| B11 | Stop | `podman stop pn1` | StopTask |
| B12 | Remove | `podman rm pn1` | Cleanup |
| B13 | Pod create | `podman pod create --name mypod` | Pod registered |
| B14 | Pod list | `podman pod ls` | Shows mypod |
| B15 | Pod inspect | `podman pod inspect mypod` | Pod details |
| B16 | Pod exists | `podman pod exists mypod` | Exit code 0 |
| B29 | Pod remove | `podman pod rm mypod` | Cleaned up with PodRmReport |
| B33 | Network ops | `podman network ls` | Shows bridge/host/none |

---

## Track C: Advanced

| # | Test | Command | Expected |
|---|------|---------|----------|
| C1 | ECR login | `aws ecr get-login-password \| docker login ...` | Succeeds |
| C2 | Tag for ECR | `docker tag alpine <ecr>:test` | Tagged |
| C3 | Push to ECR | `docker push <ecr>:test` | OCI push (or error reported) |
| C8 | Container diff | `docker diff <id>` | Error (no agent) |
| C9 | Container export | `docker export <id>` | NotImplemented |

---

## Track E: Container-to-Container Communication (ECS)

Tests real VPC networking between ECS Fargate tasks via Docker network security groups.

| # | Test | Command | Expected |
|---|------|---------|----------|
| E1 | Create network | `docker network create commsnet` | VPC SG created |
| E2 | Run nginx server | `docker run -d --name web --network commsnet nginx:alpine` | Task running |
| E3 | Get server IP | `docker inspect web` → NetworkSettings IP | Real ENI IP (10.x.x.x) |
| E4 | Curl from peer | `docker run --name client --network commsnet alpine wget -qO- http://<web-ip>` | nginx HTML response |
| E5 | Verify SG allows | Both containers should have same SG (`skls-commsnet`) | Same SG in task descriptions |
| E6 | Cross-network fail | `docker run --name isolated alpine wget -qO- --timeout=5 http://<web-ip>` | Timeout (no shared SG) |
| E7 | Cleanup | `docker rm -f web client isolated && docker network rm commsnet` | SG deleted |

## Track F: Podman Pods on ECS

Tests multi-container ECS tasks driven by Podman pod API.

| # | Test | Command | Expected |
|---|------|---------|----------|
| F1 | Create pod | `podman pod create --name mypod` | Pod registered |
| F2 | Add nginx | `podman create --pod mypod --name svc1 nginx:alpine` | Associated |
| F3 | Add sidecar | `podman create --pod mypod --name svc2 alpine sleep 3600` | Associated |
| F4 | Pod inspect | `podman pod inspect mypod` | Shows svc1 + svc2 |
| F5 | Start svc1 | `podman start svc1` | Deferred (not all started) |
| F6 | Start svc2 | `podman start svc2` | Triggers multi-container ECS task |
| F7 | Single ECS task | `aws ecs list-tasks` | 1 task for both containers |
| F8 | Task definition | `aws ecs describe-task-definition` | 2 container definitions |
| F9 | Both running | `podman ps` | svc1 + svc2 Up |
| F10 | Localhost comms | From svc2: `wget -qO- http://localhost:80` | nginx response (shared netns) |
| F11 | Pod stop | `podman pod stop mypod` | Both stopped |
| F12 | Pod remove | `podman pod rm mypod` | Cleaned up |

---

## Track G: Docker Compose

Tests `docker compose` against the ECS backend. Requires a `docker-compose.yml` in a temp directory.

```yaml
# /tmp/compose-test/docker-compose.yml
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
  worker:
    image: alpine
    command: ["sleep", "3600"]
    depends_on:
      - web
```

| # | Test | Command | Expected |
|---|------|---------|----------|
| G1 | Compose up | `DOCKER_HOST=tcp://localhost:9100 docker compose up -d` | Both services created and started |
| G2 | Compose ps | `docker compose ps` | web: running, worker: running |
| G3 | Compose logs | `docker compose logs web` | nginx access logs |
| G4 | Compose exec | `docker compose exec worker echo hello` | "hello" (requires agent) |
| G5 | Compose stop | `docker compose stop` | Both stopped |
| G6 | Compose down | `docker compose down` | Containers + network removed |
| G7 | Compose down -v | `docker compose down -v` | Containers + network + volumes removed |

## Track H: Podman Compose

Tests `podman-compose` against the ECS backend via the Podman connection.

```yaml
# /tmp/podman-compose-test/docker-compose.yml
services:
  api:
    image: nginx:alpine
  cache:
    image: alpine
    command: ["sleep", "3600"]
```

| # | Test | Command | Expected |
|---|------|---------|----------|
| H1 | Compose up | `podman-compose --podman-args="--connection=sockerless-test" up -d` | Both created |
| H2 | Compose ps | `podman-compose ps` | Both running |
| H3 | Compose down | `podman-compose down` | Cleaned up |

---

## Track I: Stateless Backend Verification

Verifies that the backend has zero local state — all container info comes from the cloud.

| # | Test | Steps | Expected |
|---|------|-------|----------|
| I1 | Create+start | `docker run -d --name persist1 nginx:alpine` | Running on Fargate |
| I2 | Verify running | `docker ps` | Shows persist1 |
| I3 | **Kill backend** | `pkill sockerless-backend-ecs` | Backend exits |
| I4 | **Restart backend** | Start backend again with same config | Backend starts fresh |
| I5 | **Verify state survived** | `docker ps` | Shows persist1 still running (from cloud) |
| I6 | **Inspect after restart** | `docker inspect persist1` | Full container details from ECS |
| I7 | **Stop after restart** | `docker stop persist1` | StopTask works (task ARN from cloud) |
| I8 | **Verify stopped** | `docker ps -a` | Shows persist1 Exited |
| I9 | Clean | `docker rm persist1` | Cleaned |

---

## Track D: Lambda-specific

| # | Test | Command | Expected |
|---|------|---------|----------|
| D1 | System info | `docker info` (on :9200) | Driver=lambda |
| D2 | Pull + create | `docker create --name l1 alpine echo hello` | Created |
| D3 | Start | `docker start l1` | Lambda invocation |
| D4 | Logs | `docker logs l1` (after delay) | Invocation output |
| D5 | Exit code | `docker inspect l1 --format {{.State.ExitCode}}` | 0 on success |
| D6 | Error propagation | `docker run --name l2 alpine false` | ExitCode=1, State.Error set |
| D7 | Env vars | `docker run -e KEY=val alpine env` | KEY=val in output |

---

## Teardown

```bash
# Stop running tasks
source aws.sh
for task in $(aws ecs list-tasks --cluster sockerless-live --region eu-west-1 --query 'taskArns[]' --output text); do
  aws ecs stop-task --cluster sockerless-live --task "$task" --region eu-west-1
done
sleep 60

# Destroy ECS infrastructure
cd terraform/environments/ecs/live && terragrunt destroy -auto-approve

# Destroy Lambda infrastructure
cd terraform/environments/lambda/live && terragrunt destroy -auto-approve

# Deregister orphaned task definitions
for td in $(aws ecs list-task-definitions --region eu-west-1 --status ACTIVE \
  --query 'taskDefinitionArns[?contains(@,`sockerless`)]' --output text); do
  aws ecs deregister-task-definition --task-definition "$td" --region eu-west-1
done

# Remove S3 state bucket
aws s3 rb s3://sockerless-tf-state --region eu-west-1 --force
```

## Known Limitations

- CloudWatch log latency (2-10s before logs appear)
- Exec requires agent or SSM (not tested without agent setup)
- CPU/memory rounded to nearest valid Fargate tier (ECS)
- Lambda max 15 minutes, no long-running containers
- NAT Gateway cost ~$0.045/hr while infrastructure exists
