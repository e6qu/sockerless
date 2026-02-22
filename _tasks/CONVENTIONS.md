# Task Conventions

> Standards for all Sockerless tasks — Definition of Done, Acceptance Criteria format, coding conventions, and testing strategy.

---

## Task File Format

Every task file follows this template:

```markdown
# COMPONENT-NNN: Title

**Component:** <component name>
**Phase:** <1|2|3|4>
**Depends on:** <task IDs or "—">
**Estimated effort:** <S|M|L|XL>

---

## Description
<What this task accomplishes, in plain language. Written so a developer
unfamiliar with the project can understand the scope.>

## Context
<Inline excerpts from the spec relevant to this task. Self-contained —
the developer should NOT need to read other documents to start working.>

## Acceptance Criteria
<Numbered, testable criteria. Each criterion is pass/fail.>

### Example Requests
<curl commands with expected responses for endpoint tasks.>

## Definition of Done
<Standard DoD checklist — same for every task, plus task-specific items.>

## Suggested File Paths
<Where code should live. Not prescriptive about internal structure.>

## Notes
<Implementation hints, edge cases, things to watch out for.>
```

---

## Component Prefixes

| Prefix | Component | Go Module Path |
|--------|-----------|----------------|
| `API` | Shared internal API types | `github.com/sockerless/api` |
| `FE` | Docker REST API frontend | `github.com/sockerless/frontend` |
| `MEM` | In-memory backend | `github.com/sockerless/backend-memory` |
| `DKR` | Docker daemon backend | `github.com/sockerless/backend-docker` |
| `AG` | Container agent | `github.com/sockerless/agent` |
| `ECS` | AWS ECS Fargate backend | `github.com/sockerless/backend-ecs` |
| `GCR` | Google Cloud Run backend | `github.com/sockerless/backend-cloudrun` |
| `ACA` | Azure Container Apps backend | `github.com/sockerless/backend-aca` |
| `LAM` | AWS Lambda backend | `github.com/sockerless/backend-lambda` |
| `GCF` | Google Cloud Functions backend | `github.com/sockerless/backend-gcf` |
| `AZF` | Azure Functions backend | `github.com/sockerless/backend-azf` |
| `TST` | Test infrastructure and suites | `github.com/sockerless/tests` |
| `TF` | Terraform infrastructure modules | `terraform/` |
| `SIM` | Cloud service simulators | `simulators/` |
| `CORE` | Shared backend core library | `github.com/sockerless/backend-core` |

---

## Definition of Done (Universal)

Every task must satisfy ALL of the following before it can be marked complete:

### Code Quality
- [ ] Code compiles: `go build ./...` passes with zero errors
- [ ] Lint passes: `golangci-lint run ./...` with zero warnings
- [ ] Vet passes: `go vet ./...` with zero warnings
- [ ] Race detector: `go test -race ./...` passes
- [ ] No new lint warnings introduced

### Testing
- [ ] All existing black-box REST tests continue to pass
- [ ] New black-box REST tests written for new/changed behavior (in `tests/` module)
- [ ] Tests run against memory backend by default
- [ ] Tests are deterministic (no flaky tests)

### Error Handling
- [ ] Custom error types used for all domain errors (not bare `errors.New`)
- [ ] Errors wrapped with context: `fmt.Errorf("operation: %w", err)`
- [ ] Zerolog structured logging on all error paths
- [ ] HTTP error responses use Docker's format: `{"message": "..."}`

### Documentation
- [ ] GoDoc comments on all exported types, functions, methods, and constants
- [ ] Module `README.md` created or updated (if this task creates or modifies a module)

### Integration
- [ ] No breaking changes to the internal API contract (unless the task explicitly requires it)
- [ ] Capability model updated if new backend capabilities are added

---

## Acceptance Criteria Standards

### For Endpoint Tasks

Each endpoint task must include:

1. **HTTP method and path** with all supported query parameters
2. **Request body** (JSON, with all fields that CI runners actually send)
3. **Response status code** and body shape
4. **Error cases** with expected status codes and messages
5. **curl examples** showing happy path and at least one error case
6. **Streaming behavior** (if applicable — logs, attach, exec)

Example AC format:
```
1. `POST /containers/create` accepts a JSON body with at minimum `Image` field
2. Returns 201 with `{"Id": "<64-char-hex>", "Warnings": []}`
3. Returns 404 if the image has not been pulled
4. Returns 409 if a container with the same name already exists
5. The created container appears in `GET /containers/json?all=true`
6. Container state is "created" in `GET /containers/{id}/json`
```

### For Backend Tasks

Each backend task must include:

1. **Internal API endpoints** implemented (which operations from the backend interface)
2. **Cloud API calls** made (specific SDK methods or REST calls)
3. **State management** (what is persisted and where)
4. **Error mapping** (cloud errors → Docker-compatible error responses)
5. **Capability reporting** (what capabilities this backend reports)

### For Agent Tasks

Each agent task must include:

1. **WebSocket message types** handled
2. **Process management** behavior (fork, exec, signal handling)
3. **Stream protocol** compliance (multiplexed framing)
4. **Failure modes** and recovery behavior

### For Test Tasks

Each test task must include:

1. **Endpoints tested** with specific scenarios
2. **Happy path and error path** coverage
3. **Test independence** (each test cleans up after itself)
4. **Backend compatibility** (tests must pass against memory backend; note any backend-specific tests)

---

## Coding Conventions

### Go Version and Modules
- **Go 1.23+** minimum
- Module paths: `github.com/sockerless/<module-name>`
- Each component is a separate Go module with its own `go.mod`
- No component imports another component's code (communicate via internal API)

### Error Handling
- Use **custom error types** for domain errors:
  ```go
  type NotFoundError struct {
      Resource string // "container", "image", "network", "volume"
      ID       string
  }
  func (e *NotFoundError) Error() string {
      return fmt.Sprintf("No such %s: %s", e.Resource, e.ID)
  }
  ```
- Wrap errors with context: `fmt.Errorf("create container: %w", err)`
- Use `errors.Is` / `errors.As` for error checking
- Map domain errors to HTTP status codes in the frontend

### Logging
- Use **zerolog** (`github.com/rs/zerolog`) for all structured logging
- Log levels: `debug` for internal state, `info` for operations, `warn` for recoverable issues, `error` for failures
- Include context in log entries:
  ```go
  log.Info().Str("container_id", id).Str("image", image).Msg("container created")
  ```
- Never log sensitive data (credentials, tokens, secrets)

### Testing Strategy
- **Black-box REST tests only** — all tests in the `tests/` module
- Tests hit the HTTP endpoint (via Unix socket or TCP)
- Test binary accepts a `--socket` or `--addr` flag to target any backend
- Default: run against memory backend for speed
- Use `testing.T` with subtests (`t.Run`)
- Table-driven style for parameterized scenarios
- Each test creates its own resources and cleans up (no shared state between tests)
- Test helpers in `tests/helpers/` for common operations (create container, pull image, etc.)

### HTTP/API Conventions
- Docker API responses use Docker's exact JSON field names (PascalCase)
- Error responses: `{"message": "<descriptive error>"}` with appropriate HTTP status
- Streaming responses use chunked transfer encoding
- Connection hijacking uses `httputil.Hijacker` interface
- API version prefix `/v1.44/` is optional — unversioned paths treated as v1.44

### Project Structure Within a Module
```
<module>/
├── go.mod
├── go.sum
├── README.md          # Module overview, usage, configuration
├── <package>.go       # Main package code
├── errors.go          # Custom error types (if needed)
├── types.go           # Type definitions (if many)
└── cmd/               # Binary entrypoint (if module produces a binary)
    └── main.go
```

---

## Effort Estimates

| Size | Description | Rough Scope |
|------|-------------|-------------|
| **S** | Simple, well-defined, <100 lines of new code | CRUD endpoint, simple passthrough |
| **M** | Moderate complexity, 100-500 lines | Endpoint with filtering/streaming, backend operation with cloud SDK |
| **L** | Significant complexity, 500-1000 lines | Protocol implementation, complex cloud integration |
| **XL** | Major feature, 1000+ lines | Full backend implementation, e2e test suite |

---

## Dependency Rules

- Tasks use **hard dependencies**: a task CANNOT start until all listed dependencies are complete
- Dependencies are listed by task ID (e.g., `Depends on: API-001, FE-001`)
- Circular dependencies are not allowed
- Tasks within the same phase CAN be parallelized if they don't depend on each other
- Cross-phase dependencies are implicit (Phase N+1 tasks may depend on Phase N tasks)
