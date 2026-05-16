---
name: manual-test
description: Run sockerless's canonical manual smoke per component — real adaptor, real binary, real output. Use when verifying a fix, before claiming "this works", or when sample-capturing for docs. Pairs with adaptor-fidelity-check.
---

# Manual test

Manual tests in sockerless drive the real reference adaptor against a real running binary and assert specific output. They are the ground truth — `go test ./...` proves compile + unit correctness; manual tests prove the **wire contract**.

## When this skill applies

- Before claiming a fix works ("I changed X" → "I ran Y and got Z").
- Before capturing sample output for a README per Phase 157.
- After context compaction, before continuing a multi-turn change.
- When CI surfaces a wire-level bug (the manual flow reproduces it locally).

## Discipline

- **No mocks.** Real binary, real adaptor.
- **Real captured output.** Paste exactly what the terminal showed; never paraphrase.
- **Round-trip per change.** If you edited the response handler, you must re-run the adaptor and see the new response.
- **Cleanup at the end.** `pkill`, `docker rm`, `gh repo delete`, etc.

## Per-component recipes

### `backends/docker`

```bash
cd backends/docker && make build
./sockerless-backend-docker -addr :13375 --log-level error > /tmp/db.log 2>&1 &
sleep 1

# Round-trip
DOCKER_HOST=tcp://localhost:13375 docker version --format '{{.Client.Version}} client / {{.Server.Version}} server'
DOCKER_HOST=tcp://localhost:13375 docker run --rm alpine:3.20 echo "hello"
curl -sS http://localhost:13375/_ping

# Cleanup
pkill -f sockerless-backend-docker
```

### `backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}`

```bash
# 1. Start the matching simulator
make sim-<cloud>-up    # aws | gcp | azure

# 2. Build + start the backend
cd backends/<name> && make build
SOCKERLESS_<CLOUD>_SIM_ENDPOINT=http://localhost:<sim-port> \
  ./sockerless-backend-<name> -addr :3375 &

# 3. Drive via docker (the frontend adaptor)
DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine echo hi

# 4. (also) drive via the cloud adaptor against the SIMULATOR to confirm the
#    cloud side of the action was reproduced fully
aws --endpoint-url http://localhost:<sim-port> ecs list-tasks --cluster <cluster>
gcloud --api-endpoint-overrides http://localhost:<sim-port> run jobs list
az --endpoint http://localhost:<sim-port> containerapp list

# 5. Cleanup
pkill -f sockerless-backend-<name>; make sim-<cloud>-down
```

### `simulators/{aws,gcp,azure}`

```bash
# 1. Start
cd simulators/<cloud> && make build && ./sockerless-simulator-<cloud> --addr :<port> &

# 2. Hit it with all three adaptor types
aws --endpoint-url http://localhost:<port> <service> <verb>
# (or gcloud / az equivalents)
# Then via SDK in a one-off Go program; then via Terraform provider with endpoint override.

# 3. Cleanup
pkill -f sockerless-simulator-<cloud>
```

### `bleephub`

```bash
# Quick start (5-step canonical recipe from bleephub/README.md)
cd bleephub && make build

openssl req -x509 -newkey rsa:2048 -days 1 -nodes \
  -keyout /tmp/bph.key -out /tmp/bph.crt \
  -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /tmp/bph.crt   # macOS

sudo BPH_TLS_CERT=/tmp/bph.crt BPH_TLS_KEY=/tmp/bph.key \
  ./sockerless-bleephub --addr :443 &

echo "ghp_0000000000000000000000000000000000000000" \
  | gh auth login --hostname localhost --with-token
export GH_HOST=localhost

# Round-trip
gh repo create demo --public
gh issue create --repo admin/demo --title "test" --body "manual smoke"
gh issue list --repo admin/demo

# Or the full Docker harness:
make bleephub-gh-docker-test
```

### Full e2e (sim + backend + runner)

```bash
make stack-aws-ecs-up                  # sim + backend + bleephub
make e2e-github-aws-ecs                # official actions/runner end-to-end
make stack-aws-ecs-down
```

## What "real captured output" looks like

```
$ DOCKER_HOST=tcp://localhost:13375 docker run --rm alpine:3.20 echo hello
hello

$ gh repo create demo --public
✓ Created repository admin/demo on GitHub
```

vs. paraphrased / made-up:

```
✗ "docker run completes successfully and prints the message"
✗ "gh repo create returns the created repo URL"
```

If you cannot show the literal terminal output, you have not actually tested it.

## When to file a bug

If any manual test fails, before fixing:

1. Capture the failing command + output verbatim.
2. Add a one-liner to `BUGS.md` under Open (next sequential BUG-NNN, severity P0–P3).
3. Include the fix shape (one sentence on where + how) once understood.
4. Then fix it in the same session (per `memory/feedback_manual_test_cycle.md`).

## Output

When this skill fires, name the component + adaptor you're about to drive, run the recipe, and paste the actual terminal output (or note what failed). End with cleanup status ("backend killed", "sim down", "repo deleted").
