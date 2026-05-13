# Go type-strengthening research

> **Status: research only.** This document collects approaches; adoption decisions are deferred. Each approach must be evaluated against sockerless's existing code shape (130k lines, 7 backends, generated Docker-API types, heavy `map[string]interface{}` at JSON boundaries) and explicit non-goals (no synthetic shims, no "just enough type safety" half-measures).

## Why this matters here

BUG-991 and BUG-992 are the canonical motivating cases. `handleContainerWait` and `handleImageList` in `backends/core/handle_*.go` each read directly from `s.Store.Containers` / `s.Store.Images` instead of dispatching through `s.self.<Method>()`. The compiler had no objection: `*Server` and `*BaseServer` share a struct, and "look at the store" vs. "ask the per-backend implementation" are both valid Go. The result was a passthrough backend (`backends/docker`) silently returning empty lists while the upstream daemon held the real state. The bug class is "two valid Go programs, only one of which is the truth — and the type system can't tell the difference." This is the territory where stronger typing pays off: encode the contract ("handlers must call `self`, never the store directly") so violating it is a build failure, not a manual-test surprise.

Sockerless also has a 3,300-line `api/openapi.yaml` driving `api/types_gen.go`, and `bleephub/` re-implements the GitHub REST + GraphQL surfaces — both spec-driven domains where the gap between "the spec says X" and "the code happens to match X" is exactly the surface stronger types can close.

## Surveyed sources

- <https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/> — Alexis King's foundational "Parse, Don't Validate" article; quoted for the validation-vs-parsing distinction.
- <https://encore.dev/blog/go-1.18-generic-identifiers> — Encore's writeup of typed identifiers using Go 1.18 generics + phantom-type-style parameters.
- <https://github.com/uber-go/nilaway> — Uber's NilAway static analyzer; Uber blog companion at <https://www.uber.com/en-NL/blog/nilaway-practical-nil-panic-detection-for-go/>.
- <https://github.com/nishanths/exhaustive> and <https://pkg.go.dev/github.com/nishanths/exhaustive> — exhaustive switch linter for enum-like constants.
- <https://github.com/alecthomas/go-check-sumtype> — `gochecksumtype` linter for sealed-interface exhaustiveness.
- <https://github.com/oapi-codegen/oapi-codegen> — OpenAPI 3 → Go server/client/types generator.
- <https://github.com/99designs/gqlgen> and <https://gqlgen.com/> — schema-first GraphQL code generation.
- <https://github.com/jmattheis/goverter> and <https://goverter.jmattheis.de/> — type-safe converter generator (already used in `backends/docker`).
- <https://github.com/ashanbrown/forbidigo> — identifier-banning linter (suitable for blocking raw `interface{}` / `any` / `map[string]any`).
- <https://github.com/sashamelentyev/usestdlibvars> — flags string-literal HTTP methods/statuses where typed `http.Method*` / `http.Status*` exist.
- <https://github.com/flyingmutant/rapid> and <https://pkg.go.dev/pgregory.net/rapid> — modern property-based testing with shrinking + state-machine generators.
- <https://github.com/leanovate/gopter> — Haskell-QuickCheck-style property tester for Go.
- <https://golangci-lint.run/docs/linters/> — meta-linter catalog; the orchestration layer for most of the above.
- <https://eli.thegreenplace.net/2018/go-and-algebraic-data-types/> — Eli Bendersky on sum-types via sealed interfaces.
- <https://medium.com/stupid-gopher-tricks/ensuring-go-interface-satisfaction-at-compile-time-1ed158e8fa17> — `var _ Interface = (*Impl)(nil)` idiom.
- <https://commaok.xyz/post/compile-time-assertions/> — additional compile-time assertion variants.
- <https://forum.golangbridge.org/t/go-builder-generator-with-compile-time-required-field-enforcement/41759> — gobetter / step-builder pattern for required-field enforcement.
- <https://dev.to/gabrielanhaia/type-driven-domain-design-in-go-encoding-invariants-at-compile-time-497i> — survey of parse-don't-validate / states-as-types / phantom types in Go.
- <https://pkg.go.dev/github.com/go-playground/validator/v10> — struct-tag-driven runtime validation.

## Approach 1: typed string IDs (newtypes + generics)

**What it is.** Replace `string` (or `[20]byte`) container/image/network IDs with a defined Go type — either a per-resource newtype (`type ContainerID string`), or a generic carrier (`type ID[T any] struct { v string }`) parameterised by a phantom marker. The compiler then rejects code that passes a `ContainerID` where an `ImageID` is expected.

**Library / tool.** Stdlib only for the newtype variant. For the generic-phantom variant, see `go.jetpack.io/typeid/typed` and Encore's pattern.

**Where it'd apply in sockerless.** Everywhere `string` flows through `api.Backend` for resource identity — `ContainerStart(id string)`, `ImageInspect(ref string)`, `NetworkConnect(net, container string)`, the Cloud Map / ECS task ARN strings inside the per-backend cloud-state code, and the `bleephub` repo / installation / run IDs. Especially valuable on the cloud-state side, where ARN-vs-task-ID-vs-container-name mismatches are a real ongoing source of confusion.

**Cost.** Mechanical refactor; touches probably every file in `backends/`. Conversions at JSON boundaries (`json.Unmarshal` into a `ContainerID`) just work because the underlying type is `string`. Go's `type T1 T2` form (not `type T1 = T2`) is what's needed — aliases don't create a distinct type ([source](https://perfects.engineering/blog/go_alias_vs_new_types/)).

**Risk.** Volume: 7 backends × ~10 ID-shaped methods × multiple call sites is a large diff, and conversion noise (`string(id)`) creeps into formatting and logging. For passthrough (`backends/docker`) the IDs are opaque upstream-issued strings, so they must round-trip byte-for-byte; a newtype is fine, a parsing constructor is not.

**Verdict for sockerless.** Try, scoped — start with the cloud-state code in `backends/{ecs,lambda,cloudrun,gcf,aca,azf}/` where ARN-vs-name-vs-ID confusion is concrete. Defer the API surface until the cloud-state experiment lands.

**Source.** > "the compiler will treat `ID[App]` and `ID[Trace]` as two distinct types" — Encore, "How we used Go 1.18 generics when designing our Identifiers" (<https://encore.dev/blog/go-1.18-generic-identifiers>). > "Type aliases were introduced to support gradual code repair while moving a type between packages during large-scale refactoring. New type declarations are perfect for creating domain-specific types, adding methods, or enforcing stricter type safety." (<https://perfects.engineering/blog/go_alias_vs_new_types/>)

## Approach 2: exhaustive switch checking on enums

**What it is.** A static analyzer that verifies every named-constant value of an enum-like type is handled in `switch` (and optionally `map` literal) cases. Catches the "added a new state, forgot to update the switch" regression.

**Library / tool.** [`github.com/nishanths/exhaustive`](https://github.com/nishanths/exhaustive) — integrated in `golangci-lint` as the `exhaustive` linter.

**Where it'd apply in sockerless.** `core.WaitCondition` (`not-running` / `next-exit` / `removed`), the cloud-state lifecycle enums per backend (Lambda InvocationState, ECS LastStatus, Cloud Run Job Execution phase), HTTP method / status code dispatch in the docker handlers, and the `filters` enum in `container_list.go`. Especially useful for the per-backend cloud-state-mapper code where each backend translates a cloud-specific enum into a normalised Docker container state.

**Cost.** Zero generator config. Add to `golangci-lint` config and watch the warnings. Tag the linter on relevant types via `//exhaustive:enforce` comments or rely on package-level defaults.

**Risk.** The linter requires "an underlying type of float, string, or integer (includes byte and rune); and has at least one constant of its type defined in the same block" — typed string constants count. Default cases don't satisfy exhaustiveness unless `-default-signifies-exhaustive` is set, which the upstream docs call "counter to the purpose."

**Verdict for sockerless.** Adopt. Cheap, additive, no codegen.

**Source.** > "an enum type is any named type that: has an underlying type of float, string, or integer (includes byte and rune); and has at least one constant of its type defined in the same block" — `pkg.go.dev/github.com/nishanths/exhaustive`. > "By default, the existence of a default case does NOT unconditionally make a switch statement exhaustive." (same page)

## Approach 3: generics-based constraints (Go 1.18+)

**What it is.** Use type parameters with interface constraints (`[T constraint]`) so functions are statically specialised for each concrete type without `interface{}` boxing. Useful for collection helpers, filters, and the converter / mapper layer.

**Library / tool.** Language feature; no library needed. `golang.org/x/exp/constraints` for `Ordered` etc., though Go 1.21+ has these in stdlib.

**Where it'd apply in sockerless.** `Store.Containers` / `Store.Images` / `Store.Networks` could share a single `Store[T Resource]` shape with a `Resource` constraint instead of three near-identical structs. The filter logic in `container_filter_test.go` / `image_filter.go` / `network_filter.go` is the obvious candidate.

**Cost.** Modest. Generics are well-supported since 1.18 but the compiler does not allow method sets in union constraints. > "A union element with more than one term may not contain an interface type with a non-empty method set" — Go 1.18 release notes (<https://tip.golang.org/doc/go1.18>). This rules out the "sum-type-via-union-constraint" trick that Rust/Haskell users may reach for.

**Risk.** Generics readability cost is real. The Encore writeup notes operator/method limits on unions; you cannot type-switch on a type parameter, which limits how clever generic dispatch can get. > "Type switches on type parameters are not supported" (<https://blog.merovius.de/posts/2024-01-05_constraining_complexity/>).

**Verdict for sockerless.** Try (narrow scope) — start with the filter helpers and the `Store` triplet. Don't try to push generics into `api.Backend`.

**Source.** Go 1.18 release notes (above) and merovius.de on generics constraint limits.

## Approach 4: nil-safety via NilAway

**What it is.** Static analyzer (analysis.Analyzer plugin) that propagates nilability through inter-procedural flows. Catches dereferences of values that some upstream path can leave nil. Closest Go gets to a borrow-checker-style guarantee without an annotation language.

**Library / tool.** [`github.com/uber-go/nilaway`](https://github.com/uber-go/nilaway).

**Where it'd apply in sockerless.** Every `*string`, `*int64`, `*Pod`, `*Container` pointer in `api/types_gen.go` (generated optional fields), the `BaseServer.self` pointer, the cloud SDK response pointers (`*ecs.DescribeTasksOutput`, `*lambda.GetFunctionOutput`, etc. — these are *all* pointer-of-pointer-fields shaped, the exact territory NilAway is built for).

**Cost.** Run as a separate analyzer or `golangci-lint` plugin. Uber reports < 5% build-time overhead. Per-package opt-in via `include-pkgs` is recommended; otherwise the noise from third-party deps is overwhelming.

**Risk.** > "NilAway is currently under active development: false positives and breaking changes can happen." — NilAway README (<https://github.com/uber-go/nilaway>). > "It is practical: it does not prevent all possible nil panics in your code, but it catches most of the potential nil panics we have observed in production" (same README). Soundness is intentionally not the goal, so it will miss real bugs. False positives need per-line annotations or scope exclusion.

**Verdict for sockerless.** Try, scoped to `backends/core/` and the per-backend cloud-state files. Skip `simulators/` — too much generated boilerplate.

**Source.** README quotes above; Uber engineering blog at <https://www.uber.com/en-NL/blog/nilaway-practical-nil-panic-detection-for-go/>.

## Approach 5: spec-driven code generation (OpenAPI / OAS)

**What it is.** Take `api/openapi.yaml` (3,300 lines) and `bleephub/`'s GitHub-API spec, generate typed handlers/clients/types. Failures of the implementation to match the spec become compile errors instead of integration-test surprises.

**Library / tool.** [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen). Sockerless already uses `goverter` (separate role: in-memory mapping, not wire-format) and the `api/` package already has `types_gen.go` — so codegen is in the toolchain, just not driving handler dispatch.

**Where it'd apply in sockerless.** `api/openapi.yaml` already exists. Currently it generates types only; oapi-codegen can also generate Echo / Chi / gorilla/mux / net/http server interfaces with typed request/response objects. `bleephub/` is the second target: GitHub publishes a maintained OpenAPI spec.

**Cost.** Adds a `go generate` step (sockerless already has these). Switching the docker REST handler layer in `backends/core/handle_*.go` from "free-form HTTP handlers reading `r.URL.Query()`" to "generated typed handler interface" is the work — every handler signature changes. CI adds the generator + `go build` round-trip; per nikita_rykhlov's DEV post, that's seconds, not minutes.

**Risk.** oapi-codegen explicitly notes its scope limits: > "the package tries to be too simple rather than too generic, making some design decisions in favor of simplicity, knowing that strongly typed Go code cannot be generated for all possible OpenAPI Schemas." (<https://github.com/oapi-codegen/oapi-codegen>). Docker's API uses a few oddities (multi-content-type response shapes; the `/containers/{id}/attach` upgrade) that may need manual handler shims around the generated interface. Schemas with `oneOf`/`anyOf` map to `interface{}` and require manual discriminator work.

**Verdict for sockerless.** Adopt for `bleephub/` (clean greenfield against a public spec). Defer for `backends/core/handle_*.go` until BUG-991/992-class issues are scoped — the win is large but the migration is the full handler tree.

**Source.** oapi-codegen README quote above; nikita_rykhlov's overview at <https://dev.to/nikita_rykhlov/go-tools-code-generation-from-openapi-specs-in-go-with-oapi-codegen-3jc1>.

## Approach 6: static analysis composition (golangci-lint with curated linters)

**What it is.** A meta-linter that orchestrates 50+ individual analyzers behind one binary, one config, one CI invocation. The leverage is in the curated set, not the individual tools.

**Library / tool.** [`golangci-lint`](https://golangci-lint.run/docs/linters/).

**Where it'd apply in sockerless.** Every Go module. Recommended baseline drawn from production setups: `staticcheck, gosimple, govet, errcheck, gosec, revive, gocyclo, misspell, unconvert, unparam, ineffassign, unused, gofmt, goimports`. Add the type-strengthening linters from this document: `exhaustive`, `gochecksumtype`, `forbidigo`, `usestdlibvars`, `nilness`, `prealloc`, `nilaway` (separate runner).

**Cost.** Single `.golangci.yml` file. CI cost is one process per module; on a 130k-line repo, expect ~30-90 seconds per module on a warm cache. Initial onboarding produces a flood of warnings — schedule a sweep, not a "fix everything" push.

**Risk.** False positives. > "There are reported cases of ineffassign producing false positives" but > "one experienced developer noted they have yet to see ineffassign or errcheck produce false positives, and they very frequently catch buggy code" (<http://rski.github.io/2020/07/17/golangci-lint.html>). Choosing too many linters is the actual mistake — pick the curated baseline above and add gradually.

**Verdict for sockerless.** Adopt. Single biggest typing-discipline lever per hour of operator time.

**Source.** golangci-lint linters page; freshman.tech / glukhov.org guides.

## Approach 7: property-based testing

**What it is.** Instead of writing example-based tests, write properties (`forall xs, sort(sort(xs)) == sort(xs)`) and let the library generate inputs. When a property fails, the framework shrinks to a minimal reproducer.

**Library / tool.** [`pgregory.net/rapid`](https://pkg.go.dev/pgregory.net/rapid) and [`github.com/leanovate/gopter`](https://github.com/leanovate/gopter).

**Where it'd apply in sockerless.** The Docker-API filter logic (container list filters, image filters) is a perfect target: input is a list of containers + a filter map, output is a filtered list, properties are obvious (idempotent, monotone in filter relaxation, etc.). Also: `goverter` converters in `backends/docker/` — for any `A → B → A` round-trip, equality should hold or differ only in named field projections. Also: the `compose_compat` parser.

**Cost.** Library dependency only. Tests live next to existing `_test.go`; `rapid.Check` integrates with `*testing.T`. Per the README, rapid runs 100 inputs by default.

**Risk.** Property-based tests are slow to write and can become "test the implementation against itself" if not careful. State-machine tests (rapid's `StateMachine`) are powerful for testing `BaseServer` lifecycle but require modelling expected behaviour separately.

**Verdict for sockerless.** Try — start with the filter logic that BUG-992 exposed as a 100-line mess. The property "delegating to `s.self.ImageList(opts)` then filtering yields the same result as a stand-alone reference filter" is testable.

**Source.** > "Compared to gopter, rapid provides a much simpler API, is much smarter about data generation and is able to minimize failing test cases fully automatically, without any user code." — rapid README (<https://github.com/flyingmutant/rapid>).

## Approach 8: compile-time interface satisfaction proofs

**What it is.** The `var _ api.Backend = (*Server)(nil)` idiom: declare a zero-memory variable whose declared type is the interface and whose value is a typed nil of the implementation. The compiler enforces method-set satisfaction at build time.

**Library / tool.** None — language idiom.

**Where it'd apply in sockerless.** Already used at the per-backend layer per `MEMORY.md` ("Compiler-enforced: `var _ api.Backend = (*Server)(nil)`"). Extending: every driver in `backends/core/drivers.go` (`DriverSet`, `AgentDriver`, `ProcessDriver`, `SyntheticDriver`), every cloud-state mapper, every UI-adapter boundary.

**Cost.** Zero. One line per implementation point. Underscore name means the variable is dropped; typed nil pointer means no allocation.

**Risk.** None. The pattern is universally idiomatic. The Uber Go Style Guide considered codifying it: <https://github.com/uber-go/guide/issues/25>.

**Verdict for sockerless.** Adopt as policy — every interface implementer in `backends/*/`, `agent/`, `simulators/*/` should carry an assertion. Add a lint rule (custom revive rule or `analyze` pass) that requires the assertion for any package implementing a `core` interface.

**Source.** > "this line ensures that your Implementation satisfies your Interface, and will fail to compile if the Interface adds methods that the Implementation fails to satisfy. Since the variable is named an underscore, it will not be kept around and won't take up any memory" — Mat Ryer / "Stupid Gopher Tricks" (<https://medium.com/stupid-gopher-tricks/ensuring-go-interface-satisfaction-at-compile-time-1ed158e8fa17>).

## Approach 9: phantom types for state machines

**What it is.** Carry an unused (phantom) type parameter that tracks an object's state at the type level. A `*Container[Running]` and a `*Container[Stopped]` are different types; methods are defined only on the states that admit them (`func (c *Container[Running]) Stop() *Container[Stopped]`).

**Library / tool.** Language feature (Go 1.18+ generics). No library; just type-parameterised structs.

**Where it'd apply in sockerless.** The container lifecycle (`Created → Running → Paused → Stopped → Removed`). The bleephub workflow run state machine (`Queued → In Progress → Completed`). The pod-materialization state (per `docs/POD_MATERIALIZATION.md`): `Spec → Materialized → Bound → Live → Torn-down`.

**Cost.** Significant. Every state transition becomes a new typed method. Existing storage / serialization needs to erase the phantom parameter (`Container[any]` or a plain `Container`) because JSON has no state-parameter representation. Probably means a parsing layer on read.

**Risk.** Adds cognitive load — readers have to grok phantom types. > "Phantom types add a small cognitive load, as engineers need to understand that Length[Meters] and Length[Feet] are different types because of the type parameter, though a short comment at the type definition usually settles this." (<https://dev.to/gabrielanhaia/type-driven-domain-design-in-go-encoding-invariants-at-compile-time-497i>). For passthrough backends where state is upstream's truth, a local phantom-typed model is fiction — it can desynchronize from upstream and lie convincingly.

**Verdict for sockerless.** Skip for the API surface (passthrough lies); unsure for `agent/` lifecycle (single-owner state). Don't pursue until typed IDs (Approach 1) lands.

**Source.** Dev.to quote above; Encore on phantom-type-style identifiers (<https://encore.dev/blog/go-1.18-generic-identifiers>).

## Approach 10: sum types via sealed interfaces (+ gochecksumtype)

**What it is.** Idiomatic Go's substitute for tagged unions: an interface with an unexported method ("sealed") restricts implementers to the same package. Adding `//sumtype:decl` and `gochecksumtype` makes type-switch exhaustiveness a compile-step check.

**Library / tool.** [`github.com/alecthomas/go-check-sumtype`](https://github.com/alecthomas/go-check-sumtype) — `gochecksumtype` in `golangci-lint`. Stdlib-only encoding otherwise.

**Where it'd apply in sockerless.** `core.PodSpec` carrying per-backend specialisations (`PodSpec.ECS`, `PodSpec.Lambda`, `PodSpec.CloudRun`, etc.) — today this is a struct with optional fields per cloud, which compiles even when none are set. As a sum type `BackendSpec = ECSSpec | LambdaSpec | CloudRunSpec | ...`, the "exactly one" invariant becomes type-level. Same for the runner spec (GitHub Actions vs. GitLab Runner). Same for the cloud SDK response wrappers (`SyncCreate` vs. `AsyncCreate` results).

**Cost.** Per-variant struct definition + one unexported method per variant. The `//sumtype:decl` annotation. Type switches replace the optional-field pattern.

**Risk.** > "the interface must be sealed, meaning it contains an unexported method. The tool validates this requirement and produces an error if the interface lacks a sealing method." (<https://github.com/alecthomas/go-check-sumtype>). JSON marshal/unmarshal needs custom `UnmarshalJSON` with a discriminator (typical pattern: `{"kind":"ecs","ecs":{...}}`). The discriminator is "another string field that can be wrong" — but it's now exactly one such field, instead of N optional struct pointers.

**Verdict for sockerless.** Try for `core.PodSpec` / runner spec / cloud SDK result wrappers. This is precisely the territory where Go's "valid-but-meaningless" struct shapes hurt.

**Source.** > "Since Go lacks native sum type support, an unexported method guarantees that only types defined in the same package can satisfy the interface, creating a closed set of variants at compile time. This allows the tool to verify that all possible cases are handled in type switches." (<https://github.com/alecthomas/go-check-sumtype>). Background: Eli Bendersky, "Go and Algebraic Data Types" (<https://eli.thegreenplace.net/2018/go-and-algebraic-data-types/>).

## Approach 11: visitor / functional-dispatch alternative to type switches

**What it is.** Instead of `switch v := s.(type) { case A: ... case B: ... }`, declare a `Visitor` interface with one method per variant; sum-type implementations call `v.Accept(myVisitor)`. The compiler enforces that every variant has a corresponding visitor method.

**Library / tool.** None — language idiom. (`mkunion` at <https://widmogrod.github.io/mkunion/value_proposition/> automates the boilerplate.)

**Where it'd apply in sockerless.** Same targets as Approach 10. Useful when the variant set is small (3-7) but the operations on it are many (>3). For one-off operations a type switch + `gochecksumtype` is cheaper.

**Cost.** Boilerplate per visitor (or codegen via `mkunion`). Each new method that operates on the variant set is a new visitor.

**Risk.** Verbose. > "The Visitor design pattern is equivalent to sum types" (<https://news.ycombinator.com/item?id=8798640>) but the equivalence comes at the cost of indirection for readers.

**Verdict for sockerless.** Skip unless Approach 10 is in place and a specific call site has >3 operations on the same sum type.

**Source.** HN thread above; ploeh blog on visitor-as-sum-type (<https://blog.ploeh.dk/2018/06/25/visitor-as-a-sum-type/>).

## Approach 12: banning untyped containers (`forbidigo` / `usestdlibvars`)

**What it is.** A lint rule that refuses to compile code containing literal patterns: `interface{}`, `map[string]interface{}`, raw HTTP method strings (`"GET"` when `http.MethodGet` exists), magic status codes (`500` when `http.StatusInternalServerError` exists). Forces the typed alternative at write time.

**Library / tool.** [`forbidigo`](https://github.com/ashanbrown/forbidigo) for the general-purpose rule, [`usestdlibvars`](https://github.com/sashamelentyev/usestdlibvars) for the HTTP/stdlib-constant cases.

**Where it'd apply in sockerless.** `backends/core/handle_*.go` — the docker handler layer is awash in literal HTTP statuses. `bleephub/` similarly. The `types_gen.go` `Status map[string]any` field (existing) would be a forbidigo-exempt site, but new code outside that file should not import the pattern.

**Cost.** golangci-lint config block. Per-rule allowlist for the few sites where `any` is genuinely correct (JSON pass-through, generated code).

**Risk.** Over-broad bans create lint noise without value. forbidigo > "supports an advanced mode where it uses type information to identify what an expression references through the analyze_types command line parameter" (<https://github.com/ashanbrown/forbidigo>), so type-aware rules are possible — start with that, not raw text matches.

**Verdict for sockerless.** Adopt `usestdlibvars` immediately (the most pure win on this list — zero false-positive risk on HTTP constants). Adopt `forbidigo` once a `map[string]interface{}` audit pass settles which sites are deliberate and which are accidents.

**Source.** > "Instead of http.NewRequest(\"GET\", \"\", nil) you can have http.NewRequest(http.MethodGet, \"\", nil) — the linter will highlight it." (usestdlibvars README, <https://github.com/sashamelentyev/usestdlibvars>).

## Approach 13: parse, don't validate (constructor-discipline)

**What it is.** Turn input validation into a parsing step that returns a typed value carrying the proven invariant. After construction, no defensive checks downstream. The canonical example: `parseNonEmpty(s) (NonEmpty[T], error)` returns a type whose existence proves non-emptiness, where `validateNonEmpty(s) error` only confirms it at one point and forgets.

**Library / tool.** Stdlib only. The discipline is a code-review rule plus a constructor convention.

**Where it'd apply in sockerless.** Container-name validation (`/^[a-zA-Z0-9][a-zA-Z0-9_.-]+$/`). Image-reference parsing (`docker.io/library/alpine:latest` → registry/repo/tag triple). Cloud Map service names. Lambda function ARNs. Anywhere sockerless takes a string from the wire and uses it as a key into cloud state.

**Cost.** Per-shape constructor + per-shape type. JSON unmarshalling needs `UnmarshalJSON` to call the constructor (otherwise the typed promise is bypassed). The user's `MEMORY.md` rule "Real fixes only. No fakes, no fallbacks, no silent shims" maps to this naturally — a parser either succeeds and produces the strong type, or it fails loudly.

**Risk.** Inconsistent uptake creates a worse outcome than no uptake: half the codebase trusts the type, half re-validates "just in case," and bugs hide in the gap. Has to be enforced at boundary modules.

**Verdict for sockerless.** Adopt for the next net-new package; retrofit case-by-case.

**Source.** > "the difference between validation and parsing lies almost entirely in how information is preserved" — Alexis King, "Parse, Don't Validate" (<https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/>). > "validation happens once, at the edge, and the type carries the result inward—twelve files of defensive checks become one constructor" (<https://dev.to/gabrielanhaia/type-driven-domain-design-in-go-encoding-invariants-at-compile-time-497i>).

## Approach 14: builder pattern with compile-time required fields (gobetter)

**What it is.** Code-generated step-builders where each `WithFoo()` returns a different type that exposes only the next valid setter. Reaching `Build()` is type-level proof that all required fields were set.

**Library / tool.** [`gobetter`](https://github.com/mobiletoly/gobetter).

**Where it'd apply in sockerless.** `PodSpec` construction — today an arbitrary subset of fields can be set and the missing-field error surfaces only at first cloud call. CLI flag parsing (`cmd/sockerless/`) where a context must have name + endpoint + auth.

**Cost.** Codegen + struct annotations. Generated step-types are visible in API documentation.

**Risk.** > "FOP is a huge advantage for library maintainers because they can easily deprecate options, combine existing options, or add new options without breaking existing client code." (<https://forum.golangbridge.org/t/go-builder-generator-with-compile-time-required-field-enforcement/41759>) — the flip side is step-builders are a strict API: changing a required field reorders the chain and breaks every caller.

**Verdict for sockerless.** Skip for the API surface (too rigid). Try for `cmd/sockerless/` context construction where the required-field guarantee earns its keep.

**Source.** Forum quote above.

## Approach 15: struct-tag validation (go-playground/validator)

**What it is.** Runtime validation driven by struct field tags: `Name string \`validate:"required,min=3,max=20\"\``. Not a compile-time check, but it consolidates the validation rules next to the type and makes them inspectable.

**Library / tool.** [`go-playground/validator/v10`](https://github.com/go-playground/validator).

**Where it'd apply in sockerless.** API request types in `bleephub/` where the spec defines field constraints. CLI flag bundles. `compose.yaml` parsing in `backends/core/compose_*.go`.

**Cost.** Library dependency. Validators run on every Decode-and-Validate path; allocation-light but reflection-based.

**Risk.** Reflection-based — runtime only, so the compiler still admits invalid struct literals in tests. Doesn't address the BUG-991/992 class at all (handler-source-of-truth bugs). Drift between tags and the rest of the code is silent until the validator runs.

**Verdict for sockerless.** Skip standalone; revisit if oapi-codegen (Approach 5) is adopted — `oapi-codegen` can emit `validator/v10` tags from the OpenAPI spec, which closes the spec→runtime check loop.

**Source.** > "Package validator implements value validations for structs and individual fields based on tags" (<https://pkg.go.dev/github.com/go-playground/validator/v10>).

## What probably WON'T work for sockerless

**Fully typed `interface{}` removal in `api/types_gen.go`.** The Docker API is genuinely heterogeneous: `Status map[string]any`, `Driver-specific options` blobs, `Plugin Config`. Banning `any` here is fighting the spec. Quarantine `any` to the generated types and ban it elsewhere — don't fight the upstream API.

**Compile-time-enforced "handler must call `self.X()`" rule via the type system.** This is the BUG-991 instinct: encode the dispatch contract in types. But `BaseServer.self api.Backend` *is* the contract — what failed was discipline, not types. A linter that bans `s.Store.<X>.<Read>` outside specifically-tagged getter functions is more tractable than a type-level proof. (Custom `revive` rule or local `analysis.Analyzer`.)

**Phantom-type state machines on container lifecycle for passthrough backends.** `backends/docker` delegates state to upstream `dockerd`. A phantom-typed local model would lie. Reserve phantom types for single-owner state (the agent, perhaps; admin orchestrator state).

**Generics-based sum types.** Go's union-element constraint forbids method sets. Sum types belong in sealed interfaces + `gochecksumtype` (Approach 10), not in generic constraints. > "A union element with more than one term may not contain an interface type with a non-empty method set" — Go 1.18 release notes.

**oapi-codegen across the entire docker REST surface, in one pass.** The Docker API has too many edges (stream upgrades, multi-content-type responses, `/exec` long-poll) for a clean generated handler interface. The migration would touch every `handle_*.go` and the cost dwarfs the BUG-991/992 fix-shape. Use oapi-codegen for greenfield (`bleephub/`) and an "audit one handler at a time" plan for the docker REST surface.

**Builder pattern on the `api.Backend` interface itself.** The interface has 62 methods (per `MEMORY.md`); a step-builder construction would replace a clear method-by-method satisfaction proof (`var _ api.Backend = (*Server)(nil)`) with a generated typestate graph that nobody reads. Keep the interface flat.

**Property-based tests on cloud round-trips.** Properties want determinism; cloud APIs don't deliver it. Run rapid against in-process logic (filters, mappers, converters); don't run it against ECS / Lambda / Cloud Run.

**Per-handler exhaustive enum dispatch when the enum is the Docker API's.** The Docker daemon adds wait conditions, filter keys, etc. without warning. `exhaustive` on Docker-defined enums creates churn every Docker release. Apply the linter to *sockerless-defined* enums (cloud-state lifecycles, internal driver kinds), not to mirrored upstream enums.

## Suggested adoption order (when decisions are revisited)

1. **`var _ Interface = (*Impl)(nil)` policy** + a custom linter that requires it for every implementor of a `core` interface. (Approach 8.)
2. **`golangci-lint` baseline** with `staticcheck, gosimple, govet, errcheck, ineffassign, unused, exhaustive, gochecksumtype, usestdlibvars, unconvert`. (Approach 6 + 2 + 10 + 12.)
3. **Typed IDs for the cloud-state layer only.** Per-backend `ContainerARN`, `TaskARN`, `LambdaFunctionName`, etc. — kept out of `api.Backend` until it earns its way in. (Approach 1.)
4. **Sealed-interface sum types for `core.PodSpec` and the runner spec.** With `gochecksumtype` already in `golangci-lint` from step 2, this is the highest-leverage shape change. (Approach 10.)
5. **NilAway on `backends/core/` + cloud-state files.** Scope-limited; revisit once false-positive rate is known. (Approach 4.)
6. **Property tests on filter logic.** Direct BUG-992 follow-up. (Approach 7.)
7. **oapi-codegen for `bleephub/`** as greenfield against the GitHub public OpenAPI spec. (Approach 5.)
8. **Parse-don't-validate constructors for container names, image refs, ARNs.** Retrofit case-by-case. (Approach 13.)

Steps 1, 2, and 3 are the cheap structural wins. Step 4 is the expensive correctness win. Everything else is supporting.
