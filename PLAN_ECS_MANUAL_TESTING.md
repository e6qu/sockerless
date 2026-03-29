# ECS Manual Testing Plan

Comprehensive manual testing of Sockerless against real AWS ECS Fargate.

## Test Results (2026-03-29)

| Phase | Result | Notes |
|-------|--------|-------|
| 1. Backend Startup & System | PASS | `_ping`, `docker info`, `docker version` all work |
| 2. Image Operations | PASS (after BUG-591 fix) | Pull, list, inspect, remove work. Real config fetched from registry |
| 3. Container Lifecycle | PASS | create/start/inspect/logs/stop/remove all work against real Fargate |
| 4. Run (create+start) | PASS (with caveat) | `docker run` works but short-lived containers show no inline output (BUG-593: CloudWatch latency). `docker logs` works after delay |
| 5. Env Vars | PASS | `FOO=bar` and `BAZ=qux` visible in `docker logs` |
| 6. Networking | PASS | Network create/list/remove work. Security group created in VPC |
| 7. Volumes | PASS | Volume create/list/inspect/remove work (in-memory) |
| 8. Logs | PASS | `docker logs` returns real CloudWatch output |
| 9. Prune & Cleanup | PASS | Container prune removes stopped containers |
| 10. Exec | SKIPPED | Requires agent sidecar or SSM — not configured in this test |
| 11. AWS Verification | PASS | Real Fargate tasks visible via `aws ecs` CLI |
| 12. Error Cases | PASS | Pause returns NotImplemented, inspect nonexistent returns 404 |
| 13. Image Config | PASS (after BUG-591) | nginx Cmd/Entrypoint correct, nginx stays running |
| 14. Container Restart | NOT TESTED | Requires running long-lived container + restart |
| 15. Container Rename | NOT TESTED | |
| 16. Container Wait | NOT TESTED | |
| 17. Container Stats/Top | NOT TESTED | Requires agent |
| 18. Archive (cp) | NOT TESTED | Requires agent |
| 19. Multi-container Pods | NOT TESTED | |
| 20. Docker Compose | NOT TESTED | |
| 21. Reverse Agent Mode | NOT TESTED | |
| 22. ECR Image Push/Pull | NOT TESTED | |
| 23. Auth | NOT TESTED | |
| 24. Events Stream | NOT TESTED | |
| 25. System Disk Usage | NOT TESTED | |
| 26. Recovery & Orphans | NOT TESTED | |

### Bugs Found During Testing

- **BUG-591 (High)**: `FetchImageConfig` broken — Docker client auth passed to Docker Hub token endpoint; image config aliases not updated. Nginx exited immediately due to synthetic `/bin/sh` CMD. **Fixed.**
- **BUG-592 (Medium)**: DX — 10+ env var exports needed to start backend. Need single-command setup.
- **BUG-593 (Medium)**: DX — `docker run` for short-lived containers shows no inline output (CloudWatch latency).
- **BUG-594 (Low)**: Orphaned in-memory containers from prior sessions not cleaned up on restart.

---

## Infrastructure

Terraform applied via `terraform/environments/ecs/live/`. Resources:

| Resource | Value |
|---|---|
| ECS Cluster | `sockerless-live` |
| VPC | `vpc-06604af1d392f2a2d` |
| ECR | `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live` |
| EFS | `fs-08de10ba1cccd66ee` |
| Log Group | `/sockerless/live/containers` |
| Execution Role | `arn:aws:iam::729079515331:role/sockerless-live-execution-role` |
| Task Role | `arn:aws:iam::729079515331:role/sockerless-live-task-role` |
| Task SG | `sg-0a322f5dac3c180d3` |
| Private Subnets | `subnet-0db382ef9b8a99731,subnet-026cf23f29e6fd271` |

## Environment Variables

```bash
source aws.sh
export AWS_REGION=eu-west-1
export SOCKERLESS_ECS_CLUSTER=sockerless-live
export SOCKERLESS_ECS_SUBNETS=subnet-0db382ef9b8a99731,subnet-026cf23f29e6fd271
export SOCKERLESS_ECS_SECURITY_GROUPS=sg-0a322f5dac3c180d3
export SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::729079515331:role/sockerless-live-execution-role
export SOCKERLESS_ECS_TASK_ROLE_ARN=arn:aws:iam::729079515331:role/sockerless-live-task-role
export SOCKERLESS_ECS_LOG_GROUP=/sockerless/live/containers
export SOCKERLESS_AGENT_EFS_ID=fs-08de10ba1cccd66ee
export SOCKERLESS_ECS_PUBLIC_IP=true
```

## Prerequisites

1. **Build the ECS backend binary**
   ```bash
   cd backends/ecs && go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs
   ```

2. **Start the backend** (listens on `:9100`, serves Docker-compatible API)
   ```bash
   ./sockerless-backend-ecs -addr :9100
   ```

3. **Point Docker CLI at Sockerless**
   ```bash
   export DOCKER_HOST=tcp://localhost:9100
   ```

---

## Test Phases

### Phase 1: Backend Startup & System Endpoints

| # | Test | Command | Expected |
|---|------|---------|----------|
| 1.1 | Server starts | `./sockerless-backend-ecs -addr :9100` | Logs show listening, no errors |
| 1.2 | Docker info | `docker info` | Shows Driver=ecs-fargate, OS=AWS Fargate |
| 1.3 | Docker version | `docker version` | Shows API version 1.44, Server: Sockerless |
| 1.4 | Docker ping | `curl http://localhost:9100/_ping` | Returns `OK` |

### Phase 2: Image Operations

Images are metadata-only — ECS pulls from registries at task start. Real image config (Cmd, Entrypoint, Env) is fetched from the registry at pull time.

| # | Test | Command | Expected |
|---|------|---------|----------|
| 2.1 | Pull public image | `docker pull alpine:latest` | Succeeds, config fetched from Docker Hub |
| 2.2 | Pull nginx | `docker pull nginx:alpine` | Succeeds, Cmd=["nginx","-g","daemon off;"] |
| 2.3 | Pull ubuntu | `docker pull ubuntu:22.04` | Succeeds, Cmd=["/bin/bash"] |
| 2.4 | List images | `docker images` | Shows all pulled images with sizes |
| 2.5 | Inspect image | `docker inspect alpine:latest` | Returns Config with Cmd, Env, etc. |
| 2.6 | Tag image | `docker tag alpine:latest myalpine:v1` | Succeeds |
| 2.7 | List after tag | `docker images` | Shows both alpine:latest and myalpine:v1 |
| 2.8 | Remove image | `docker rmi myalpine:v1` | Removes the tag |
| 2.9 | Image history | `docker history alpine:latest` | Returns synthetic layer history |
| 2.10 | Image prune | `docker image prune -f` | Removes dangling images |

### Phase 3: Container Lifecycle (Core)

| # | Test | Command | Expected |
|---|------|---------|----------|
| 3.1 | Create container | `docker create --name test1 alpine:latest echo hello` | Returns container ID |
| 3.2 | Inspect created | `docker inspect test1` | State.Status="created", State.Running=false |
| 3.3 | Start container | `docker start test1` | Registers task def, calls RunTask |
| 3.4 | Inspect running | `docker inspect test1` | State.Running=true, NetworkSettings.IPAddress set |
| 3.5 | List running | `docker ps` | Shows test1 with "Up X seconds" |
| 3.6 | View logs | `docker logs test1` | Shows "hello" from CloudWatch (may need ~10s delay) |
| 3.7 | Wait for exit | (wait ~30s for pollTaskExit) | Container transitions to Exited(0) |
| 3.8 | List stopped | `docker ps -a` | Shows test1 with "Exited (0)" |
| 3.9 | Stop already stopped | `docker stop test1` | Returns NotModified (idempotent) |
| 3.10 | Remove container | `docker rm test1` | Deregisters task def, cleans up state |
| 3.11 | Remove nonexistent | `docker rm test1` | Returns 404 |

### Phase 4: Docker Run (Create+Start Combined)

| # | Test | Command | Expected |
|---|------|---------|----------|
| 4.1 | Run short-lived | `docker run --name run1 alpine:latest echo "hello world"` | Completes (output may be delayed) |
| 4.2 | Check logs | `docker logs run1` | Shows "hello world" after ~10s |
| 4.3 | Run detached | `docker run -d --name run2 nginx:alpine` | Returns container ID, runs in background |
| 4.4 | Verify running | `docker ps` | Shows run2 running |
| 4.5 | Verify still running after 30s | `sleep 30 && docker ps` | run2 still "Up" (not exiting like before BUG-591) |
| 4.6 | Force remove | `docker rm -f run2` | Stops task, removes container |
| 4.7 | Run with name conflict | `docker run --name run1 alpine:latest echo hi` | Returns Conflict error (run1 still exists) |
| 4.8 | Clean up | `docker rm run1` | Removes exited container |

### Phase 5: Environment Variables & Working Directory

| # | Test | Command | Expected |
|---|------|---------|----------|
| 5.1 | Single env var | `docker run --name e1 -e FOO=bar alpine:latest env` | Output includes FOO=bar |
| 5.2 | Multiple env vars | `docker run --name e2 -e A=1 -e B=2 -e C=3 alpine:latest env` | Shows A=1, B=2, C=3 |
| 5.3 | Working directory | `docker run --name e3 -w /tmp alpine:latest pwd` | Shows /tmp |
| 5.4 | User override | `docker run --name e4 -u nobody alpine:latest id` | Shows uid for nobody |
| 5.5 | Custom entrypoint | `docker run --name e5 --entrypoint /bin/echo alpine:latest hello` | Shows "hello" |
| 5.6 | Labels | `docker run --name e6 -l mykey=myval alpine:latest echo ok` | `docker inspect e6` shows label |

### Phase 6: Networking

| # | Test | Command | Expected |
|---|------|---------|----------|
| 6.1 | List default networks | `docker network ls` | Shows bridge, host, none |
| 6.2 | Create network | `docker network create testnet` | Returns network ID, creates VPC security group |
| 6.3 | Inspect network | `docker network inspect testnet` | Shows Driver, IPAM config |
| 6.4 | List networks | `docker network ls` | Shows testnet |
| 6.5 | Run on network | `docker run -d --name net1 --network testnet nginx:alpine` | Task uses network's security group |
| 6.6 | Inspect with container | `docker network inspect testnet` | Shows net1 in Containers |
| 6.7 | Create second network | `docker network create testnet2` | Creates second SG |
| 6.8 | Connect container | `docker network connect testnet2 net1` | Associates second SG |
| 6.9 | Disconnect container | `docker network disconnect testnet2 net1` | Removes SG association |
| 6.10 | Verify AWS SG exists | `aws ec2 describe-security-groups --filters Name=group-name,Values=skls-testnet` | Shows the SG |
| 6.11 | Stop and remove | `docker rm -f net1` | Cleans up |
| 6.12 | Remove networks | `docker network rm testnet testnet2` | Deletes VPC security groups |
| 6.13 | Verify AWS SG deleted | `aws ec2 describe-security-groups --filters Name=group-name,Values=skls-testnet` | Empty result |
| 6.14 | Network prune | `docker network prune -f` | Removes unused custom networks |

### Phase 7: Volumes

| # | Test | Command | Expected |
|---|------|---------|----------|
| 7.1 | Create volume | `docker volume create testvol` | Returns volume name |
| 7.2 | List volumes | `docker volume ls` | Shows testvol |
| 7.3 | Inspect volume | `docker volume inspect testvol` | Returns Driver, Mountpoint, labels |
| 7.4 | Run with bind mount | `docker run --name bm1 -v /tmp/test:/data alpine:latest ls /data` | Mount point exists (empty on Fargate without EFS) |
| 7.5 | Remove volume | `docker volume rm testvol` | Succeeds |
| 7.6 | Remove nonexistent | `docker volume rm nonexistent` | Returns 404 |
| 7.7 | Volume prune | `docker volume prune -f` | Removes unused volumes |

### Phase 8: Logs (CloudWatch)

| # | Test | Command | Expected |
|---|------|---------|----------|
| 8.1 | Basic logs | `docker logs <exited-id>` | Returns output from CloudWatch |
| 8.2 | Tail | `docker logs --tail 5 <id>` | Returns last 5 lines |
| 8.3 | Since | `docker logs --since 5m <id>` | Returns logs from last 5 minutes |
| 8.4 | Timestamps | `docker logs -t <id>` | Lines prefixed with RFC3339 timestamps |
| 8.5 | Follow mode | `docker logs -f <running-id>` | Streams new lines (polls CloudWatch every 1s) |
| 8.6 | Logs on never-started | `docker create --name nostartlog alpine && docker logs nostartlog` | Returns empty (BUG-585 fix: no crash) |

### Phase 9: Prune & Cleanup

| # | Test | Command | Expected |
|---|------|---------|----------|
| 9.1 | Container prune | `docker container prune -f` | Removes all stopped containers, reports space |
| 9.2 | Image prune | `docker image prune -f` | Removes dangling images |
| 9.3 | Network prune | `docker network prune -f` | Removes unused custom networks |
| 9.4 | Volume prune | `docker volume prune -f` | Removes unused volumes |
| 9.5 | System prune | `docker system prune -f` | Combines all prune operations |

### Phase 10: Exec (requires agent)

Exec requires a running agent sidecar or ECS ExecuteCommand (SSM).

| # | Test | Command | Expected |
|---|------|---------|----------|
| 10.1 | Simple exec | `docker exec <running-id> ls /` | Lists root filesystem |
| 10.2 | Exec with env | `docker exec -e MY_VAR=hello <id> env` | Shows MY_VAR=hello |
| 10.3 | Exec with workdir | `docker exec -w /tmp <id> pwd` | Shows /tmp |
| 10.4 | Interactive shell | `docker exec -it <id> sh` | Opens shell session |
| 10.5 | Exec on stopped | `docker exec <stopped-id> ls` | Returns error (container not running) |

### Phase 11: AWS Resource Verification

| # | Test | Command | Expected |
|---|------|---------|----------|
| 11.1 | List running tasks | `aws ecs list-tasks --cluster sockerless-live` | Shows task ARNs for running containers |
| 11.2 | Describe task | `aws ecs describe-tasks --cluster sockerless-live --tasks <arn>` | Shows Fargate task: RUNNING, 256 CPU, 512 MB |
| 11.3 | List task defs | `aws ecs list-task-definitions` | Shows sockerless-* definitions |
| 11.4 | Describe task def | `aws ecs describe-task-definition --task-definition <name>` | Shows container image, env vars, log config |
| 11.5 | CloudWatch logs | `aws logs describe-log-streams --log-group-name /sockerless/live/containers` | Shows log streams |
| 11.6 | Get log events | `aws logs get-log-events --log-group-name ... --log-stream-name ...` | Shows actual log lines |
| 11.7 | Describe cluster | `aws ecs describe-clusters --clusters sockerless-live` | Shows runningTasksCount |
| 11.8 | List stopped tasks | `aws ecs list-tasks --cluster sockerless-live --desired-status STOPPED` | Shows stopped task ARNs |
| 11.9 | Verify task tags | `aws ecs describe-tasks ... --include TAGS` | Shows sockerless tags (container ID, backend, instance) |

### Phase 12: Error Cases

| # | Test | Command | Expected |
|---|------|---------|----------|
| 12.1 | Inspect nonexistent | `docker inspect nonexistent` | Returns 404 |
| 12.2 | Remove nonexistent | `docker rm nonexistent` | Returns 404 |
| 12.3 | Stop nonexistent | `docker stop nonexistent` | Returns 404 |
| 12.4 | Pause (unsupported) | `docker pause <running-id>` | Returns NotImplemented |
| 12.5 | Unpause (unsupported) | `docker unpause <id>` | Returns NotImplemented |
| 12.6 | Name conflict | `docker create --name dup alpine && docker create --name dup alpine` | Second returns Conflict |
| 12.7 | Remove running (no force) | `docker rm <running-id>` | Returns Conflict (must stop first or use -f) |
| 12.8 | Start already running | `docker start <running-id>` | Returns NotModified |
| 12.9 | Kill stopped | `docker kill <stopped-id>` | Returns Conflict |
| 12.10 | Logs on nonexistent | `docker logs nonexistent` | Returns 404 |

### Phase 13: Image Config Verification

Verify that real image configs are fetched from registries (BUG-591 fix).

| # | Test | Command | Expected |
|---|------|---------|----------|
| 13.1 | Alpine config | `docker pull alpine && docker inspect alpine` | Cmd=["/bin/sh"] |
| 13.2 | Nginx config | `docker pull nginx:alpine && docker inspect nginx:alpine` | Cmd=["nginx","-g","daemon off;"], Entrypoint=["/docker-entrypoint.sh"] |
| 13.3 | Redis config | `docker pull redis:alpine && docker inspect redis:alpine` | Cmd=["redis-server"], Entrypoint=["docker-entrypoint.sh"] |
| 13.4 | Python config | `docker pull python:3-alpine && docker inspect python:3-alpine` | Cmd=["python3"] |
| 13.5 | Nginx stays running | `docker run -d --name nginx-up nginx:alpine && sleep 60 && docker ps` | nginx-up still "Up" |
| 13.6 | Redis stays running | `docker run -d --name redis-up redis:alpine && sleep 60 && docker ps` | redis-up still "Up" |

### Phase 14: Container Restart

| # | Test | Command | Expected |
|---|------|---------|----------|
| 14.1 | Start long-lived | `docker run -d --name restart-test nginx:alpine` | Running on Fargate |
| 14.2 | Verify task ARN | `aws ecs list-tasks --cluster sockerless-live` | Note the task ARN |
| 14.3 | Restart | `docker restart restart-test` | Stops old task, starts new task (BUG-587 fix) |
| 14.4 | Verify new task | `aws ecs list-tasks --cluster sockerless-live` | Different task ARN than step 14.2 |
| 14.5 | Still running | `docker ps` | restart-test still "Up" |
| 14.6 | Clean up | `docker rm -f restart-test` | Stops and removes |

### Phase 15: Container Rename

| # | Test | Command | Expected |
|---|------|---------|----------|
| 15.1 | Create | `docker create --name orig alpine:latest echo hi` | Returns ID |
| 15.2 | Rename | `docker rename orig renamed` | Succeeds |
| 15.3 | Inspect by new name | `docker inspect renamed` | Returns container, Name="/renamed" |
| 15.4 | Old name gone | `docker inspect orig` | Returns 404 |
| 15.5 | Clean up | `docker rm renamed` | Succeeds |

### Phase 16: Container Wait

| # | Test | Command | Expected |
|---|------|---------|----------|
| 16.1 | Run and wait | `docker run -d --name wait-test alpine:latest sleep 5` | Returns ID |
| 16.2 | Wait for exit | `docker wait wait-test` | Blocks until exit, returns exit code 0 |
| 16.3 | Clean up | `docker rm wait-test` | Succeeds |

### Phase 17: Container Stats & Top (requires agent)

| # | Test | Command | Expected |
|---|------|---------|----------|
| 17.1 | Stats (no stream) | `docker stats --no-stream <running-id>` | Returns synthetic CPU/memory stats |
| 17.2 | Top | `docker top <running-id>` | Returns process list (via agent or synthetic) |

### Phase 18: Archive/Copy (requires agent)

| # | Test | Command | Expected |
|---|------|---------|----------|
| 18.1 | Copy to container | `docker cp /tmp/test.txt <running-id>:/tmp/` | Succeeds (via agent) |
| 18.2 | Copy from container | `docker cp <running-id>:/etc/hostname /tmp/out.txt` | Succeeds (via agent) |
| 18.3 | Stat path | `curl -I http://localhost:9100/containers/<id>/archive?path=/etc` | Returns file metadata |

### Phase 19: Multi-container Pods

| # | Test | Command | Expected |
|---|------|---------|----------|
| 19.1 | Create pod | (via API: create pod, add 2 containers) | Pod created |
| 19.2 | Start pod | `docker start <container1>` (triggers deferred start for all) | Single ECS task with 2 container definitions |
| 19.3 | Verify single task | `aws ecs list-tasks --cluster sockerless-live` | Only 1 task for both containers |
| 19.4 | Clean up | Remove both containers | Task stopped, definitions deregistered |

### Phase 20: Docker Compose (basic)

| # | Test | Command | Expected |
|---|------|---------|----------|
| 20.1 | Compose up | `docker compose -f test-compose.yml up -d` | Services created as Fargate tasks |
| 20.2 | Compose ps | `docker compose -f test-compose.yml ps` | Shows running services |
| 20.3 | Compose logs | `docker compose -f test-compose.yml logs` | Shows CloudWatch logs |
| 20.4 | Compose down | `docker compose -f test-compose.yml down` | Stops and removes all |

### Phase 21: Reverse Agent Mode

| # | Test | Command | Expected |
|---|------|---------|----------|
| 21.1 | Set callback URL | `export SOCKERLESS_CALLBACK_URL=http://<host>:9100` | Backend configured for reverse agent |
| 21.2 | Run with reverse agent | `docker run -d --name rev1 nginx:alpine` | Agent dials back to backend |
| 21.3 | Exec via reverse | `docker exec rev1 ls /` | Executes via reverse WebSocket |
| 21.4 | Clean up | `docker rm -f rev1` | Stops task, agent disconnects |

### Phase 22: ECR Image Push/Pull

| # | Test | Command | Expected |
|---|------|---------|----------|
| 22.1 | Login to ECR | `aws ecr get-login-password \| docker login --username AWS --password-stdin <ecr-url>` | Login succeeded |
| 22.2 | Tag for ECR | `docker tag alpine:latest <ecr-url>:test` | Succeeds |
| 22.3 | Push to ECR | `docker push <ecr-url>:test` | Syncs to ECR |
| 22.4 | Pull from ECR | `docker pull <ecr-url>:test` | Uses ECR auth |
| 22.5 | Run from ECR | `docker run --name ecr1 <ecr-url>:test echo hi` | Task pulls from ECR |

### Phase 23: Auth

| # | Test | Command | Expected |
|---|------|---------|----------|
| 23.1 | Auth check | `curl -X POST http://localhost:9100/auth -d '{"username":"test"}'` | Returns auth status |

### Phase 24: Events Stream

| # | Test | Command | Expected |
|---|------|---------|----------|
| 24.1 | Start events | `docker events &` | Starts listening |
| 24.2 | Create container | `docker create --name evt1 alpine echo hi` | Events stream shows "create" event |
| 24.3 | Start container | `docker start evt1` | Events stream shows "start" event |
| 24.4 | Clean up | `docker rm evt1` | Events stream shows "die", "destroy" events |

### Phase 25: System Disk Usage

| # | Test | Command | Expected |
|---|------|---------|----------|
| 25.1 | System df | `docker system df` | Shows Images, Containers, Volumes usage |
| 25.2 | System df verbose | `docker system df -v` | Shows detailed breakdown |

### Phase 26: Recovery & Orphaned Resources

| # | Test | Command | Expected |
|---|------|---------|----------|
| 26.1 | Create running container | `docker run -d --name orphan nginx:alpine` | Task running on Fargate |
| 26.2 | Kill backend (SIGKILL) | `kill -9 <backend-pid>` | Backend dies, Fargate task continues |
| 26.3 | Restart backend | `./sockerless-backend-ecs -addr :9100` | Backend recovers registry |
| 26.4 | List containers | `docker ps -a` | Should show recovered container |
| 26.5 | Verify task still running | `aws ecs list-tasks --cluster sockerless-live` | Task ARN matches |
| 26.6 | Clean up | `docker rm -f orphan` | Stops Fargate task |

---

## Teardown

### Stop the backend
```bash
# Ctrl+C the backend process
```

### Destroy all AWS infrastructure
```bash
cd terraform/environments/ecs/live
source aws.sh
terragrunt destroy -auto-approve
```

### Clean up orphaned resources (if destroy misses anything)
```bash
# Check for leftover task definitions
aws ecs list-task-definitions --region eu-west-1 --query 'taskDefinitionArns[?contains(@,`sockerless`)]'

# Deregister any remaining
aws ecs deregister-task-definition --task-definition <arn> --region eu-west-1

# Check for leftover security groups (from docker network create)
aws ec2 describe-security-groups --region eu-west-1 --filters "Name=group-name,Values=skls-*"

# Delete any remaining
aws ec2 delete-security-group --group-id <sg-id> --region eu-west-1

# Check for leftover Cloud Map namespaces
aws servicediscovery list-namespaces --region eu-west-1

# Delete the state bucket (after destroy)
aws s3 rb s3://sockerless-tf-state --region eu-west-1 --force
```

---

## Known Limitations

- **No container pause/unpause** — ECS Fargate doesn't support this
- **Images are synthetic** — ECS pulls from registry at task start; real config (Cmd, Entrypoint) fetched during `docker pull`
- **Exec requires agent or SSM** — Without agent sidecar, exec falls back to synthetic (empty output)
- **CloudWatch log latency** — 2-10 seconds before logs appear; `docker run` for short-lived containers shows no inline output
- **Bind mounts** — EFS-backed when `SOCKERLESS_AGENT_EFS_ID` is set; otherwise empty scratch volumes
- **Networks are partial** — Docker networks create VPC security groups for isolation, but no L2 networking
- **NAT Gateway cost** — ~$0.045/hr while infrastructure exists; destroy promptly after testing
- **Tasks in private subnets** — Need `SOCKERLESS_ECS_PUBLIC_IP=true` or NAT Gateway for image pulls
- **CPU/Memory hardcoded** — All tasks use 256 mCPU / 512 MB (Fargate minimum)
- **Single security group per container** — `ECSState.SecurityGroupID` only holds one network SG; connecting to multiple networks overwrites
