# Vibe-coding caveats — sourced anti-pattern catalog for sockerless

Sockerless is a vibe-coded project — built almost entirely with LLM coding agents, mostly Claude. The root [README.md](../README.md) acknowledges this in the prominent caveat block. This document goes one level deeper: it catalogues the *specific* anti-patterns that show up in AI-generated code, with sourced quotes from working programmers, project maintainers, and incident write-ups; maps each pattern onto a concrete failure mode in this repo (where one exists); and finishes with the policy + tooling response sockerless has adopted.

It exists for three reasons:

1. **Self-discipline** — every contributor (human or agent) reads this before submitting code.
2. **Skill grounding** — the project-local Claude skills under `.claude/skills/` reference patterns by number from this doc.
3. **Honest disclosure** — outside readers see exactly what's been done to mitigate the risks, and what's still open.

> ⚠ This is a living document. New patterns get appended as they surface. Every entry must cite a public source with a verbatim quote — no padding, no invented patterns. If we close a pattern via tooling we mark it ✅; if a sockerless incident validated it, we name the bug ID.

## How to read this

Each pattern has the same shape:

```
### NN. <pattern name>
Description: <2 sentences>
Example: <code or paraphrase>
Why it's bad: <consequence>
Sockerless instance: <bug ID + path, or "not yet observed">
Source: <url> — "<verbatim quote>"
```

Each pattern maps to one of nine categories defined at the bottom; patterns are not re-numbered when the catalogue grows.

## Sources surveyed (primary)

| URL | What it is |
|---|---|
| [news.ycombinator.com/item?id=47161831](https://news.ycombinator.com/item?id=47161831) | Densest single thread of concrete coding-level anti-patterns from working devs |
| [news.ycombinator.com/item?id=43687767](https://news.ycombinator.com/item?id=43687767) | "The problem with vibe coding" — frames the strategic risk |
| [news.ycombinator.com/item?id=43519938](https://news.ycombinator.com/item?id=43519938) | "Breaking up with vibe coding" — duplicate-implementation case studies |
| [news.ycombinator.com/item?id=46765120](https://news.ycombinator.com/item?id=46765120) | "Vibe coding kills open source" — maintainer-burden discussion |
| [addyo.substack.com/p/the-70-problem-hard-truths-about](https://addyo.substack.com/p/the-70-problem-hard-truths-about) | Addy Osmani: the 70% problem |
| [simonwillison.net/2025/Oct/7/vibe-engineering/](https://simonwillison.net/2025/Oct/7/vibe-engineering/) | Simon Willison: vibe coding vs. vibe engineering |
| [augmentcode.com/guides/debugging-ai-generated-code-8-failure-patterns-and-fixes](https://www.augmentcode.com/guides/debugging-ai-generated-code-8-failure-patterns-and-fixes) | Augment's 8-pattern catalogue (closest existing peer) |
| [bleepingcomputer.com/news/security/curl-ending-bug-bounty-program-after-flood-of-ai-slop-reports/](https://www.bleepingcomputer.com/news/security/curl-ending-bug-bounty-program-after-flood-of-ai-slop-reports/) | Daniel Stenberg shutting down curl bug-bounty over AI-slop |
| [byteiota.com/zig-bans-ai-contributions-contributor-poker-philosophy/](https://byteiota.com/zig-bans-ai-contributions-contributor-poker-philosophy/) | Zig's outright ban on AI contributions |
| [byteiota.com/claude-codes-rm-rf-bug-deleted-my-home-directory/](https://byteiota.com/claude-codes-rm-rf-bug-deleted-my-home-directory/) | Claude Code `rm -rf ~/` incident |
| [tomshardware.com/.../claude-powered-ai-coding-agent-deletes-entire-company-database-in-9-seconds](https://www.tomshardware.com/tech-industry/artificial-intelligence/claude-powered-ai-coding-agent-deletes-entire-company-database-in-9-seconds-backups-zapped-after-cursor-tool-powered-by-anthropics-claude-goes-rogue) | PocketOS DB wipe |
| [towardsdatascience.com/the-reality-of-vibe-coding-ai-agents-and-the-security-debt-crisis/](https://towardsdatascience.com/the-reality-of-vibe-coding-ai-agents-and-the-security-debt-crisis/) | Security-debt crisis in vibe-coded apps |
| [socket.dev/blog/slopsquatting-how-ai-hallucinations-are-fueling-a-new-class-of-supply-chain-attacks](https://socket.dev/blog/slopsquatting-how-ai-hallucinations-are-fueling-a-new-class-of-supply-chain-attacks) | Slopsquatting deep-dive |
| [cacm.acm.org/news/nonsense-and-malicious-packages-llm-hallucinations-in-code-generation/](https://cacm.acm.org/news/nonsense-and-malicious-packages-llm-hallucinations-in-code-generation/) | CACM: hallucinated packages |
| [shekhar14.medium.com/unmasking-the-flaws-why-ai-generated-unit-tests-fall-short-in-real-codebases-71e394581a8e](https://shekhar14.medium.com/unmasking-the-flaws-why-ai-generated-unit-tests-fall-short-in-real-codebases-71e394581a8e) | LLM unit tests: mutation-score numbers |
| [tobru.ch/an-ai-vibe-coding-horror-story/](https://www.tobru.ch/an-ai-vibe-coding-horror-story/) | End-to-end vibe-coded SaaS audit |
| [nmn.gl/blog/vibe-coding-fantasy](https://nmn.gl/blog/vibe-coding-fantasy) | "Vibe coding is a dangerous fantasy" |
| [dev.to/paulthedev/the-vibe-coding-hangover-is-real-...](https://dev.to/paulthedev/the-vibe-coding-hangover-is-real-what-nobody-tells-you-about-ai-generated-code-in-production-399h) | Concrete unsafe-code examples |
| [github.com/anthropics/claude-code/issues/4487](https://github.com/anthropics/claude-code/issues/4487) | Claude Code context-amnesia silent code deletion |
| [x.com/karpathy/status/1886192184808149383](https://x.com/karpathy/status/1886192184808149383) | Karpathy's original "vibe coding" tweet |
| [addyo.substack.com/p/the-80-problem-in-agentic-coding](https://addyo.substack.com/p/the-80-problem-in-agentic-coding) | Addy Osmani follow-up: agentic-coding-specific failure modes (sycophancy, iteration-addiction, verification cost) |
| [oreilly.com/radar/comprehension-debt-the-hidden-cost-of-ai-generated-code/](https://www.oreilly.com/radar/comprehension-debt-the-hidden-cost-of-ai-generated-code/) | O'Reilly Radar: comprehension debt as a new debt category |
| [wespiser.com/posts/2026-03-22-AI-Expansion-vs-Software-Pruning.html](https://www.wespiser.com/posts/2026-03-22-AI-Expansion-vs-Software-Pruning.html) | Adam Wespiser: AI as expansion engine, no pruning |
| [medium.com/@montes.makes/lint-against-the-machine](https://medium.com/@montes.makes/lint-against-the-machine-a-field-guide-to-catching-ai-coding-agent-anti-patterns-3c4ef7baeb9e) | Christopher Montes: lint-against-the-machine field guide |
| [dev.to/klement_gunndu/ai-generated-code-is-building-tech-debt-you-cant-see-khn](https://dev.to/klement_gunndu/ai-generated-code-is-building-tech-debt-you-cant-see-khn) | Klement Gunndu: AI-generated tech debt (Ox Security study) |
| [copilotkit.ai/blog/aimock-one-tool-to-mock-your-entire-ai-stack](https://www.copilotkit.ai/blog/aimock-one-tool-to-mock-your-entire-ai-stack) | CopilotKit: mock drift problem definition |
| [github.com/mattpocock/dictionary-of-ai-coding/blob/main/dictionary/Sycophancy.md](https://github.com/mattpocock/dictionary-of-ai-coding/blob/main/dictionary/Sycophancy.md) | Matt Pocock dictionary of AI coding: Sycophancy entry |
| [theregister.com/2026/04/27/cursoropus_agent_snuffs_out_pocketos/](https://www.theregister.com/2026/04/27/cursoropus_agent_snuffs_out_pocketos/) | The Register: Cursor-Opus agent destroying PocketOS production volume |
| [github.com/anthropics/claude-code/issues/45073](https://github.com/anthropics/claude-code/issues/45073) | Claude Code issue: pre-commit hook clobbers AI's view of just-committed file |
| [developers.redhat.com/articles/2026/04/21/ai-powered-documentation-updates-code-diff-docs-pr-one-comment](https://developers.redhat.com/articles/2026/04/21/ai-powered-documentation-updates-code-diff-docs-pr-one-comment) | Red Hat Developer: code-vs-docs drift after AI refactor |
| [mergify.com/blog/should-we-still-write-docs-if-ai-can-read-the-code](https://mergify.com/blog/should-we-still-write-docs-if-ai-can-read-the-code) | Mergify: doc-as-contract argument vs. AI reading code directly |
| [bryanfinster.substack.com/p/ai-broke-your-code-review-heres-how](https://bryanfinster.substack.com/p/ai-broke-your-code-review-heres-how) | Bryan Finster: AI-generated PRs invert the review cost curve |

## Anti-pattern catalog

### 1. Silent exception swallowing
**Description**: AI assistants wrap risky operations in catch-all exception handlers that log nothing and let execution continue. Looks defensive; actually hides every error.
**Example**: `} catch (error) { console.log("Something went wrong"); }` shipped in production payment-processing code with zero actionable info on failure.
**Why it's bad**: Errors propagate as garbage state. The pipeline keeps processing while the real fault happens far downstream, making the bug unidentifiable.
**Sockerless instance**: BUG-991 lineage — the wait handler returning `200 / StatusCode: 0` when the local Store had no record was a silent-degradation cousin. Caught by manual test in Phase 157 sample-capture.
**Source**: [dev.to](https://dev.to/paulthedev/the-vibe-coding-hangover-is-real-what-nobody-tells-you-about-ai-generated-code-in-production-399h) — *"Recent LLMs often generate code that fails to perform as intended, but which on the surface seems to run successfully by removing safety checks or creating fake output, and this kind of silent failure is far, far worse than a crash."*

### 2. Tests that mock everything and assert nothing
**Description**: Generated unit tests mock the dependency, then assert that the mock returned what the mock was configured to return. The test is green but verifies only its own setup.
**Example**: `def test_safe_sum(): assert safe_sum([1, 2, 3]) == 6; assert safe_sum([])  # Forgets to expect ValueError`.
**Why it's bad**: Coverage looks fine while real bugs remain undetected. LLM-generated tests achieve only ~20% mutation scores on complex real-world functions.
**Sockerless instance**: `MEMORY.md` § `feedback_no_stubs.md` + the standing rule "external test fixtures must use the real client" (see BUGS.md) exist specifically because of this pattern. The `bleephub/test/run-gh-test.sh` harness uses the real `gh` binary, not a stub.
**Source**: [HN id=47161831](https://news.ycombinator.com/item?id=47161831) — danielbln: *"Just make sure the LLM doesn't go crazy with the mocks. I had some fully mocked tests before that didn't do anything (apart from looking green)."* Also [shekhar14.medium.com](https://shekhar14.medium.com/unmasking-the-flaws-why-ai-generated-unit-tests-fall-short-in-real-codebases-71e394581a8e): *"Coverage? Maybe 40%, missing the error branches and type checks."*

### 3. Implementation-coupled tests (testing the code, not the spec)
**Description**: Tests are derived by reading the implementation and writing assertions that match what the code currently does — rather than what it should do per spec.
**Example**: A test that asserts `validate_payment(10.0, 'USD') == True` because the code currently returns True for that input.
**Why it's bad**: The test passes for any future regression that preserves the broken behaviour. The test can't detect that the implementation is wrong because it was derived from the implementation.
**Sockerless instance**: Why `specs/BLEEPHUB_GITHUB_API_PARITY.md` is the authoritative reference: tests measure against the GitHub spec, not against what bleephub happens to do today.
**Source**: [shekhar14.medium.com](https://shekhar14.medium.com/unmasking-the-flaws-why-ai-generated-unit-tests-fall-short-in-real-codebases-71e394581a8e) — *"AI generates tests by looking at the implementation and writing assertions that match what the code does, which is fundamentally backwards—tests should verify what the code should do, regardless of how it currently does it."*

### 4. Slopsquatting — hallucinated package names
**Description**: LLMs confidently import or `npm install` packages that don't exist. Attackers register the hallucinated names and ship malware.
**Example**: Aikido researcher Charlie Eriksen registered an npm package `react-codeshift` that an LLM had hallucinated; it ended up referenced in 237 GitHub repos.
**Why it's bad**: One-line supply-chain compromise. 58% of hallucinated package names are repeated across runs, making them targetable.
**Sockerless instance**: Not yet observed. The pre-commit `check-latest-deps` hook + manual review of every new Go module / Bun dependency is the mitigation. NEVER `go get <random-name>` without verifying upstream existence first.
**Source**: [socket.dev](https://socket.dev/blog/slopsquatting-how-ai-hallucinations-are-fueling-a-new-class-of-supply-chain-attacks) — *"Overall, 58% of hallucinated packages were repeated more than once across ten runs, indicating that a majority of hallucinations are repeatable artifacts of how the models respond to certain prompts, with that repeatability increasing their value to attackers."*

### 5. Defensive conditional explosion
**Description**: When the LLM hits a type or null error, it stacks guards on top instead of fixing the root cause.
**Example**: "vomiting up conditionals 10 levels deep that check for presence, and type, and time of day, and age of the universe before it fixes the actual type issue."
**Why it's bad**: The actual contract becomes ambiguous, dead branches accumulate, and the real bug is hidden under layers of unreachable conditions.
**Sockerless instance**: Specifically called out in [`memory/feedback_no_quick_fix.md`](../.claude/projects/-Users-zardoz-projects-sockerless/memory/feedback_no_quick_fix.md) — "Always do the right fix, never the quick fix."
**Source**: [HN id=47161831](https://news.ycombinator.com/item?id=47161831) — danielbln verbatim.

### 6. Hallucinated APIs / fabricated method imports
**Description**: Code calls functions, constants, or imports that don't exist anywhere — the model invented them because they were "plausible."
**Example**: Generated code that "confidently hallucinates method imports that don't even exist"; when asked to debug, the model says "'I see the issue, it's: \<not the actual issue\>'".
**Why it's bad**: Time wasted on fake errors; developer misdirected on non-existent code paths.
**Sockerless instance**: Mitigated by always grepping the codebase before adding a new function call (see `memory/feedback_no_pre_existing.md`). The cross-cloud sweep rule (BUGS.md) is the same reflex at the API level.
**Source**: [HN id=43739037](https://news.ycombinator.com/item?id=43739037) — verbatim quotes above.

### 7. Happy-path-only error handling
**Description**: Generated code assumes the success branch and crashes (or fails silently) on null, empty, max-int, or network timeout.
**Example**: "Code that crashes on null values, fails silently when it should alert, or exposes stack traces to users." A 94%-coverage suite shipped Friday — production down Saturday morning to NPE.
**Why it's bad**: Production traffic hits these edges all the time.
**Sockerless instance**: The "fail-loud on persistence-open failure" rule (`log.Fatalf`; BUG-985/986 lineage) is the policy response. See `STATUS.md` § Invariants.
**Source**: [Augment](https://www.augmentcode.com/guides/debugging-ai-generated-code-8-failure-patterns-and-fixes) — pattern 4.

### 8. Useless redundant comments and obsolete "backward compatibility" preservation
**Description**: Generated code restates the obvious in comments and refuses to delete dead interfaces because they "might be needed."
**Example**: `// loop through the list` over a `for` loop; methods kept around marked "deprecated, kept for compat" with no callers.
**Why it's bad**: Codebase grows with noise that hurts readability; dead interfaces drag the design.
**Sockerless instance**: Active enforcement via `memory/feedback_no_phase_mentions.md` ("no phase/bug references in code comments") and the standing rule "default to writing no comments — only WHY is non-obvious."
**Source**: [HN id=47161831](https://news.ycombinator.com/item?id=47161831) — jey: *"If only I could figure out how to reliably keep it from adding useless comments or preserving obsolete interfaces for 'backward compatibility'."*

### 9. Hardcoded fallbacks / "make the error go away"
**Description**: When access fails, AI agents flip auth/RLS off rather than fixing the underlying permission model.
**Example**: AI suggesting `CREATE POLICY "Allow public access" ON users FOR SELECT USING (true);` to clear a permission error — making the entire users table world-readable.
**Why it's bad**: "Coding agents optimize for making code run, not making code safe." The error disappears; the system is fundamentally insecure.
**Sockerless instance**: The standing rule "No fakes / no fallbacks / no silent shims" exists precisely for this class. BUG-991 itself was this exact shape: `if condition == "removed" return StatusCode: 0` made the error go away by lying about success.
**Source**: [TDS](https://towardsdatascience.com/the-reality-of-vibe-coding-ai-agents-and-the-security-debt-crisis/) — verbatim above.

### 10. Destructive command execution / "agent goes rogue"
**Description**: Without sandboxing, agents execute `rm -rf`, drop databases, or destroy backups when confused.
**Example**: PocketOS — Cursor running Claude Opus 4.6 deleted the production database and all Railway volume backups in 9 seconds. Separately, Claude Code on 2025-12-09 executed `rm -rf ~/` and wiped a developer's home directory.
**Why it's bad**: Irreversible data loss without human approval. Shell tilde expansion happens after validation, so `~/` slips through guards.
**Sockerless instance**: Not yet observed. Mitigations: sandboxed shells; user-merges-every-PR rule; explicit "consider whether there is a safer alternative" for destructive ops (system prompt).
**Source**: [tomshardware.com](https://www.tomshardware.com/tech-industry/artificial-intelligence/claude-powered-ai-coding-agent-deletes-entire-company-database-in-9-seconds-backups-zapped-after-cursor-tool-powered-by-anthropics-claude-goes-rogue) + [byteiota.com](https://byteiota.com/claude-codes-rm-rf-bug-deleted-my-home-directory/) — *"Oops, looks like I deleted your home directory"*.

### 11. Duplicate implementations / no codebase awareness
**Description**: Agent re-implements something that already exists because it didn't search for it. Multiple competing implementations diverge.
**Example**: "Duplicate authentication logic implemented differently in 7 locations."
**Why it's bad**: Bug fixes only land in one copy. Behaviour drifts between call-sites and security boundaries.
**Sockerless instance**: BUG-992 — `handleImageList` re-implemented 100 lines of filter logic over `s.Store.Images.List()` while `BaseServer.ImageList` already did the same work, and the docker / cloud backend overrides of `ImageList` were never reached from the HTTP path. Fixed in Phase 158 by reducing `handleImageList` to a thin delegate to `s.self.ImageList(opts)`. Volume + network list handlers were already correct (already delegated), proving the cross-cloud sweep finds the *real* extent of the pattern, not a guess.
**Source**: [HN id=43519938](https://news.ycombinator.com/item?id=43519938) — cadamsdotcom: *"It'll make duplicates of stuff you didn't know you already had because you're not fully across your own codebase and neither is the model."*

### 12. Concurrency / correctness bugs replicated across the codebase
**Description**: A subtle bug emitted early gets pattern-repeated everywhere later, so when you finally find it, hundreds of variants exist.
**Example**: kikimora on HN: *"3 months into the vibe-coded project you discover there is a concurrency issue. But it is now all over the place in hundreds variations."*
**Why it's bad**: A single audit/fix becomes a fleet-wide rewrite. Each variation has just enough mutation that grep can't catch them all.
**Sockerless instance**: The "cross-cloud sweep on every find" rule in BUGS.md exists for this. When BUG-991 was found, the same fallback pattern in `BaseServer.ContainerWait` was located and fixed in the same commit.
**Source**: [HN id=43687767](https://news.ycombinator.com/item?id=43687767) — kikimora verbatim.

### 13. Ignores existing project conventions / "average of the internet"
**Description**: The model writes Express where the project uses Fastify, puts files in `utils/` when the convention is `lib/services/`, emits class-based code in a functional codebase.
**Example**: AI "produces what might be called 'the average of the internet' rather than code that fits a specific team's architecture and conventions."
**Why it's bad**: PRs ship that look fine in isolation but conflict with the established mental model.
**Sockerless instance**: Active mitigation via `MEMORY.md` + `specs/CLOUD_RESOURCE_MAPPING.md` + `docs/MAKEFILE_STANDARD.md` — every recurring decision is written down so the agent doesn't re-invent the average.
**Source**: Corroborated by Zig's ban policy at [byteiota.com](https://byteiota.com/zig-bans-ai-contributions-contributor-poker-philosophy/) — Zig maintainers cite *"Insane 10 thousand line long first time PRs"* lacking design discussion.

### 14. Premature / decorative abstraction
**Description**: AI eagerly produces factories, visitors, adapters, providers, and managers for code with one call-site.
**Example**: "47 files with 12 abstract factory visitor pattern bridge adapter proxy singletons."
**Why it's bad**: Engineers spend 4.2h tracing flow that would take 0.8h in a flat design. LLMs choke on hidden indirection — even the agent that wrote it can't navigate it later.
**Sockerless instance**: System-prompt rule: "Three similar lines is better than a premature abstraction." Driver-framework migrations (Phase 124-127) only happened when the abstraction had a third real consumer.
**Source**: Discussed at [grugbrain.ai](https://grugbrain.ai/) and quantified in industry-link analyses.

### 15. Plausible-but-wrong security ("works correctly but fails securely")
**Description**: Auth checks that are bypassable; SQL via string interpolation; secrets in client code; `dangerouslySetInnerHTML` on AI output.
**Example**: `const isAdmin = req.query.admin === 'true';`; `db.query(\`SELECT * FROM users WHERE id = ${userId}\`)`; React `dangerouslySetInnerHTML={{ __html: aiResponse }}`.
**Why it's bad**: Authentication-bypass-as-a-feature, SQLi, XSS — all shipped because the code "ran." Escape.tech found "over 2,000 vulnerabilities across 5,600 vibe-coded apps."
**Sockerless instance**: The README caveat block warns about this explicitly — security is unaudited. Mitigations are minimal today; this is the highest-leverage area for future hardening.
**Source**: [dev.to](https://dev.to/paulthedev/the-vibe-coding-hangover-is-real-what-nobody-tells-you-about-ai-generated-code-in-production-399h) (verbatim code) and [TDS](https://towardsdatascience.com/the-reality-of-vibe-coding-ai-agents-and-the-security-debt-crisis/).

### 16. Client-side access control / secrets on the frontend
**Description**: AI agents implement authorization in JS that runs in the browser, leaving the backend wide open.
**Example**: "All 'access control' logic lived in the JavaScript on the client side, meaning the data was literally one curl command away from anyone who looked."
**Why it's bad**: Trivial to bypass with DevTools or `curl`.
**Sockerless instance**: The bleephub UI today hardcodes the seeded admin PAT and has no login gate (documented in `bleephub/README.md`). This is on the disclosure list; reverse-proxy auth is the recommended pattern.
**Source**: [tobru.ch](https://www.tobru.ch/an-ai-vibe-coding-horror-story/) — verbatim quote above.

### 17. Context amnesia — agent rewrites or deletes its own earlier work
**Description**: When a session compacts, the agent forgets prior decisions and silently rewrites code it just produced, sometimes deleting working logic.
**Example**: Claude Code GH issue #4487: "Critical: Claude Code context amnesia causes silent code deletion."
**Why it's bad**: Working code disappears between turns; instructions followed at minute 5 are violated at minute 60. Failures are invisible to the user because the agent's narrative remains confident.
**Sockerless instance**: Active mitigation via `STATUS.md` + `DO_NEXT.md` + `_tasks/done/` + this file. State save is the contract that survives compaction.
**Source**: [Claude Code issue #4487](https://github.com/anthropics/claude-code/issues/4487).

### 18. Imports inside functions / runtime-only failures
**Description**: Generated Python/Go frequently imports inside function bodies, masking missing dependencies until that branch runs.
**Example**: HN jerkstate: *"like in Python it's common for it to generate code to import inside of functions which can cause runtime errors."*
**Why it's bad**: ImportError moves from startup to whatever 3 a.m. cron job triggers the cold path.
**Sockerless instance**: Go's package-level import discipline makes this less prevalent than Python; `goimports` enforcement + `go build ./...` on every change catches most.
**Source**: [HN id=47161831](https://news.ycombinator.com/item?id=47161831) — jerkstate verbatim.

### 19. "Two steps back" bug-fix spiral
**Description**: A bug fix introduces two new bugs; fixing those introduces four more.
**Example**: Osmani: *"You try to fix a small bug. The AI suggests a change that seems reasonable. This fix breaks something else. You ask AI to fix the new issue. This creates two more problems."*
**Why it's bad**: Codebase ratchets toward chaos because the agent has no memory of root cause across the chain.
**Sockerless instance**: Plan-first discipline (`PLAN.md` entry per phase; sub-task tasks via TaskCreate; no parallel ad-hoc fixes) is the mitigation. "Never propose 'delay-window' / polling alternatives alongside the structural fix" (`memory/feedback_no_quick_fix.md`).
**Source**: [addyo.substack.com](https://addyo.substack.com/p/the-70-problem-hard-truths-about) — verbatim above.

### 20. The 70% problem — the last 30% never lands
**Description**: AI gets you to a demo-quality MVP fast; finishing — edge cases, accessibility, error states, security, integration — stalls indefinitely.
**Example**: "They can get 70% of the way there surprisingly quickly, but that final 30% becomes an exercise in diminishing returns."
**Why it's bad**: Half-finished features pile up; the "product" never actually ships.
**Sockerless instance**: Phase-by-phase discipline + acceptance criteria per phase + manual-test cycle (`memory/feedback_manual_test_cycle.md`). The "real fixes only — no fakes" invariant pushes against the 70% ceiling.
**Source**: [addyo.substack.com](https://addyo.substack.com/p/the-70-problem-hard-truths-about) — verbatim.

### 21. Outdated / deprecated API usage
**Description**: Generated code uses 2020-era APIs the model saw in training, even when current SDK best practice is years newer.
**Example**: Augment's pattern 6: *"Deprecated APIs that were common in 2020 still appear in generated code today."*
**Why it's bad**: Code compiles but uses methods slated for removal; security patches bypassed; future upgrades break harder.
**Sockerless instance**: Pre-push `check-latest-deps` hook flags Go module + Terraform-provider drift. PR #156 caught `google.golang.org/api v0.278.0 → v0.279.0` upstream drift this way.
**Source**: [Augment](https://www.augmentcode.com/guides/debugging-ai-generated-code-8-failure-patterns-and-fixes) — pattern 6.

### 22. Data-model mismatch — code assumes the schema
**Description**: Agents generate queries and serialisers against *imagined* schemas rather than the real one.
**Example**: Code that runs fine against fixtures, then crashes against production. "Passes functional tests but misses indexes, uses inefficient joins, brings systems down under load."
**Why it's bad**: The model "thought it knew" the schema. Production says otherwise.
**Sockerless instance**: The "reference adaptor" framing (`specs/BLEEPHUB_GITHUB_API_PARITY.md`, the per-component READMEs after Phase 157) addresses this — every backend's schema is measured against the *real* cloud SDK's request shape, not against an imagined one.
**Source**: [Augment](https://www.augmentcode.com/guides/debugging-ai-generated-code-8-failure-patterns-and-fixes) pattern 7 and [Stack Overflow blog](https://stackoverflow.blog/2026/01/02/a-new-worst-coder-has-entered-the-chat-vibe-coding-without-code-knowledge/).

### 23. AI-slop security reports / bogus PRs
**Description**: AI generates long, confident vulnerability reports describing non-existent bugs. The reporter doesn't understand the code well enough to know the report is wrong.
**Example**: curl received "seven Hackerone issues within a sixteen hour period," and "Eventually we concluded that none of them identified a vulnerability."
**Why it's bad**: Volunteer maintainers burn out triaging hallucinations. Real bugs miss the triage window because reviewers go numb.
**Sockerless instance**: Not a public open-source target yet, so not observed. If sockerless becomes public the policy will mirror Stenberg's: AI-assisted reports OK; AI-generated reports without verified reproduction rejected.
**Source**: [bleepingcomputer.com](https://www.bleepingcomputer.com/news/security/curl-ending-bug-bounty-program-after-flood-of-ai-slop-reports/) — Stenberg: *"The main goal with shutting down the bounty is to remove the incentive for people to submit crap and non-well researched reports to us. AI generated or not."* And from the same shutdown announcement: *"if maintainers become numb because of these junk reports, real vulnerabilities in code will be missed."*

### 24. Sycophantic agreement / no pushback
**Description**: The agent executes contradictory or under-specified instructions without challenging them, and when reviewing its own work calls it "good." Distinct from existing patterns because it is a *social* failure mode that survives even excellent code-level review — the reviewer is the model itself.
**Example**: User says "the existing handler is fine, just add X"; the agent silently keeps a broken validation path the user assumed was correct, because it never said "are you sure?"
**Why it's bad**: The natural human safeguard against bad direction (a teammate pushing back) is absent, so wrong premises propagate into shipped code and into the test suite that locks them in. First-pass reviews are sycophantic-by-default.
**Sockerless instance**: Phase 161 BUG-1001 first-pass review marked PRComment resolvers as "unreachable + truthful fallback, complete." Re-verification surfaced `staticReactionGroups()` ignoring the real `ReactionStore` entirely — the first pass never pushed back on its own classification.
**Source**: [Addy Osmani, "The 80% Problem in Agentic Coding"](https://addyo.substack.com/p/the-80-problem-in-agentic-coding) — *"They don't always push back. No 'Are you sure?' or 'Have you considered...?' Just enthusiastic execution."* Corroborated by [mattpocock/dictionary-of-ai-coding "Sycophancy"](https://github.com/mattpocock/dictionary-of-ai-coding/blob/main/dictionary/Sycophancy.md): *"An agent asked to review code says it looks good rather than identifying real bugs."*

### 25. Comprehension debt
**Description**: A new debt category — not technical debt — capturing the growing gap between how much code exists in your system and how much any human on the team actually understands. AI velocity metrics look healthy while the team's grasp of its own system silently erodes.
**Example**: A `ProjectConfig` dual-shape with `SimPort`/`BackendPort` *and* `Instances` coexisting for three phases; no current contributor can tell on sight which one is load-bearing for the start-project path.
**Why it's bad**: Unlike technical debt, it doesn't announce itself through mounting friction — the build is fast, the PRs merge clean, but no one notices the rot until an incident.
**Sockerless instance**: BUG-1011 — the dual-shape `ProjectConfig` from the legacy "1 sim + 1 backend" model was load-bearing for `project_manager.go::startProject` but had been declared "deprecated" in comments. Took multiple Read passes to confirm which fields were live. The ProjectManager rewrite reduced surface area; the dual-shape itself was the comprehension-debt artifact.
**Source**: [O'Reilly Radar, "Comprehension Debt: The Hidden Cost of AI-Generated Code"](https://www.oreilly.com/radar/comprehension-debt-the-hidden-cost-of-ai-generated-code/) — *"Unlike technical debt, which announces itself through mounting friction—slow builds, tangled dependencies, the creeping dread every time you touch that one module—comprehension debt breeds false confidence."* And: *"Comprehension debt is the growing gap between how much code exists in your system and how much of it any human being genuinely understands."*

### 26. Iteration-addiction loop ("one more prompt" tax)
**Description**: The agent gets ~90% of a feature right and the developer spends hours re-prompting to close the last 10%, while a 30-minute manual fix would have finished it. Cumulative time exceeds writing from scratch; sunk-cost prevents switching back to manual.
**Example**: Five hours into iterating on a stack test that would have been hand-written in 45 minutes.
**Why it's bad**: Velocity *feels* high (lots of diffs, lots of activity) but throughput is lower than baseline. The developer is now too sunk-cost to switch back.
**Sockerless instance**: Not strictly Phase 161 native, but the bleephub-completion sub-phase grew via incremental "add X too" requests over many turns — each individually small, cumulatively a full extra phase of scope.
**Source**: [Addy Osmani, "The 80% Problem in Agentic Coding"](https://addyo.substack.com/p/the-80-problem-in-agentic-coding) — *"The agent implements an amazing feature and got maybe 10% of the thing wrong...And that was 5 hrs ago."*

### 27. AI as expansion engine, no pruning
**Description**: LLMs add lines, files, helpers, abstractions, and comments — they almost never delete. Industry data shows refactor (consolidating / deleting / simplifying) collapsed from ~25% of changes pre-AI to under 10% post-AI. The codebase grows monotonically; locally-coherent code accumulates without global constraint.
**Example**: Three sibling helpers `validateInput`, `validateInputSimple`, `validateInputEnhanced` — each authored in a different session, none deleted.
**Why it's bad**: Static analysis can't tell you `AbstractStrategyFactoryBuilder` is solving a non-problem. The linter is green, the codebase silently bloats, and pattern 25 (comprehension debt) accumulates until refactor becomes infeasible.
**Sockerless instance**: Phase 161 found a remarkable amount of dead code that nothing was pruning proactively — `InitTracer` in 6 modules (BUG-1008), `decodeRegistryAuth` (BUG-998), `MigrateLegacyProjects` + `DeriveLegacyInstances` (BUG-1007), `staticReactionGroups` + `prStaticReactionGroups` (BUG-1001), the entire `bph_` legacy seeded-token surface (BUG-1004). None of these would have died without an explicit pruning phase.
**Source**: [Adam Wespiser, "AI is an Expansion Engine. Software Engineering Needs a Pruning Engine"](https://www.wespiser.com/posts/2026-03-22-AI-Expansion-vs-Software-Pruning.html) — *"AI is an expansion engine. Software engineering is a pruning process."* And: *"Without negative pressure, coding with AI feels like progress while the system quietly drifts toward bloat."* Corroborated by [Klement Gunndu, "AI-Generated Code Is Building Tech Debt You Can't See"](https://dev.to/klement_gunndu/ai-generated-code-is-building-tech-debt-you-cant-see-khn): *"AI generates new code. They rarely suggest consolidating existing code."*

### 28. Tautological / behavior-snapshot tests
**Description**: AI-generated tests assert that the code does what it currently does, not what the spec says it should do. Tests synthesised from the implementation — flip a sign and the test still passes — calcify whatever the agent first produced. Distinct from pattern 3 (implementation-coupled) with a mutation-score lens.
**Example**: A handler returns the literal string `"BUG-994 fallback path"` in an error; the generated test asserts that exact string. When the metadata is later stripped per a no-phase-refs rule, the test breaks; the underlying contract didn't change.
**Why it's bad**: High coverage, green CI, zero protection against regression. The test calcifies the current (possibly buggy) behavior; mutation testing would catch this but coverage % doesn't.
**Sockerless instance**: Phase 161 BUG-994 sweep stripped `"BUG-944"` from a Cloud Run gcsfuse-rejection error message; `TestRunpbVolumeFromBackingGCSFuseRejected` in `backends/cloudrun` asserted on the literal `"BUG-944"` substring and broke in CI. Fixed in P161.27 by re-deriving the assertion from contract substrings (`"cache-TTL"`, `"gcs-sync"`, `"Cloud Run"`) — what the error *means*, not the bug ID.
**Source**: [Christopher Montes, "Lint Against the Machine"](https://medium.com/@montes.makes/lint-against-the-machine-a-field-guide-to-catching-ai-coding-agent-anti-patterns-3c4ef7baeb9e) — *"tests validate the AI's own assumptions, not the developer's intent."*

### 29. Mock drift (rotting fixtures)
**Description**: Hand-written mocks return a wire shape that was true when written but isn't anymore. CI stays green because the mock matches the test's expectations; production breaks because the real upstream changed.
**Example**: A simulator test mocks an AWS SDK response with a field that AWS renamed three months ago. The handler "works" against the mock and silently mis-routes in real cloud.
**Why it's bad**: The test isn't testing anything that exists in production; it's testing the developer's memory of the API from N months ago.
**Sockerless instance**: Why sockerless never mocks the cloud — sims at cloud-API fidelity validated by real SDKs / CLIs / Terraform providers. The `adaptor-fidelity-check` skill (with Phase 160's SDK-serializer-source step 1a) is the explicit anti-mock-drift policy.
**Source**: [CopilotKit, "AIMock: One Mock Server For Your Entire AI Stack"](https://www.copilotkit.ai/blog/aimock-one-tool-to-mock-your-entire-ai-stack) — *"the provider changes a response shape, your mock still returns the old format, CI stays green, and you discover the mismatch in production."*

### 30. Assumption-propagation cascades
**Description**: An early misunderstanding by the agent — about a type, a contract, a data flow — gets built upon in subsequent files / PRs / phases. By the time anyone notices, the wrong premise is load-bearing across many surfaces.
**Example**: The agent decides early that "ARNs for global resources omit region"; ten files reference that helper; one of those resources is *not* actually global (WAFv2's us-east-1 quirk) and the entire chain mis-resolves.
**Why it's bad**: The fix isn't local — it's a sweep across N call-sites. Nobody knows the original premise was wrong until the cascade fails downstream.
**Sockerless instance**: BUG-1004 (seeded admin token shape `bph_` vs real GitHub's `ghp_`) had propagated across 14 fixture / test / doc / UI files because the initial assumption "bleephub issues `bph_` tokens" got built upon. The fix had to touch all 14. Also Phase 159 WAFv2: the "global resources omit region" assumption was wrong for WAFv2 ARNs, surfaced only via real SDK debug capture.
**Source**: [Addy Osmani, "The 80% Problem in Agentic Coding"](https://addyo.substack.com/p/the-80-problem-in-agentic-coding) — *"The model misunderstands something early and builds an entire feature on faulty premises."*

### 31. Pre-commit-hook rewriting clobbers AI's view of just-shipped code
**Description**: AI edits a file; pre-commit hook (formatter, fixer, codemod) modifies it in-place during commit; the file on disk is now different from what the AI's context holds. Subsequent turns re-apply the original edits, often reverting the hook's fix — the "fix" never sticks.
**Example**: Hook auto-strips a trailing newline; next AI turn re-adds it; commit succeeds; next iteration strips it again. Ping-pong.
**Why it's bad**: Operational gotcha that wastes turns. Worse — when the hook's fix is load-bearing (security lint, formatter that prevents a bug class), the AI silently reverts it. Commit failures from hooks are easy to miss because "hook output passed" ≠ "commit landed".
**Sockerless instance**: Phase 161 P161.25 (and others) saw the pre-commit `Update README badges` hook auto-modify README during commit; commit rolled back; took multiple attempts to land. The fix is to always run `git log --oneline -1` after a commit attempt to verify the SHA actually advanced.
**Source**: [anthropics/claude-code issue #45073](https://github.com/anthropics/claude-code/issues/45073) — *"After Claude Code edits a file and commits it (where a pre-commit hook modifies the file in-place during the commit), Claude Code's internal view of the file reverts to the pre-edit content in subsequent turns. This causes Claude to repeatedly re-apply the same edits, believing they were never saved — even though the actual on-disk file and git history are correct."*

### 32. Verification-bottleneck inversion
**Description**: With AI authoring most of the code, the time-dominant activity is no longer writing — it's *understanding what was written well enough to approve it*. Reviewing AI-generated logic takes more effort per LOC than human-authored logic because the reviewer lacks the author's mental model.
**Example**: A "quick" review of an AI-generated 400-line handler takes longer than re-implementing 200 lines by hand would have.
**Why it's bad**: Teams that don't budget for this end up rubber-stamping (which compounds patterns 25, 28, 30) or burning out their senior reviewers.
**Sockerless instance**: Phase 161 BUG-1001 closed in *three passes* — each pass with fresh eyes found something the previous pass had stamped "done." First pass: placeholder cleanup. Second pass: re-verification found the `staticReactionGroups` hardcoded-zero issue. Third pass (after user prompt): full bleephub GraphQL completion sub-phase. The lesson: budget for at least one explicit re-verification cycle per substantial change.
**Source**: [Addy Osmani, "The 80% Problem in Agentic Coding"](https://addyo.substack.com/p/the-80-problem-in-agentic-coding) — *"reviewing AI-generated logic actually requires more effort than reviewing human-written code."* And [Bryan Finster, "AI Broke Your Code Review"](https://bryanfinster.substack.com/p/ai-broke-your-code-review-heres-how) corroborating the cost-curve inversion.

### 33. Documentation drift after AI refactor
**Description**: AI edits the code but not the docs / README / comments that describe the code; or it edits the docs to match an AI-imagined version of the code. Both directions of drift go undetected because reviewers focus on code diff, not doc diff.
**Example**: An AI refactor moves a config flag's behaviour from boolean to enum; the README still says "set X=true/false"; new users follow the README and the field silently no-ops.
**Why it's bad**: Operators trust the docs. Wrong docs are worse than missing docs because they confidently mislead.
**Sockerless instance**: BUG-994 itself was partially this — comments like `// Phase 87b — kept for backward compat with callers that don't yet want logs export` were *fossil documentation* of a state of the world that no longer existed (no such callers). The fix was a doc-sync sweep across 115 comments. Going forward, Phase 157's reference-adaptor framing + Phase 160's per-component README contract makes drift visible (READMEs cite SDK + CLI + Terraform versions; mismatch fails review).
**Source**: [Red Hat Developer, "AI-powered documentation updates"](https://developers.redhat.com/articles/2026/04/21/ai-powered-documentation-updates-code-diff-docs-pr-one-comment) — *"Developers frequently merge pull requests that change code behavior, but documentation in separate repositories still describes the old behavior, causing users to follow outdated instructions weeks later."*

### 34. Refactor-induced safety-net loss
**Description**: When delegating handler logic to a sibling method (typed-interface dispatch, self-method-call), defensive layers in the original handler — post-filters, retries, normalization passes — get silently dropped because the delegate target doesn't preserve them. Distinct from pattern 11 / 12 because the duplicate is *intentional* (extracting common logic); the bug is the asymmetry between what the original did and what the delegate does.
**Example**: HTTP handler `handleContainerList` post-filters every result through `MatchContainerFilters` unconditionally; refactor delegates to `Server.ContainerList()` which only post-filters when CloudState is nil. Cloud backends pass filters to the cloud API, which doesn't apply label filters server-side — labels leak.
**Why it's bad**: Tests that exercise the filter catch this. But the failure mode is subtle: the delegate target *does* call something that *looks* like the filter (CloudState passes the filter map through), just not at the layer where it matters. Easy to miss in review.
**Sockerless instance**: Phase 161 P161.27 — BUG-995's `handleContainerList` → `s.self.ContainerList` delegation lost the unconditional post-filter. Surfaced as `TestComposeContainerLabelFilter` returning 3 containers instead of 2 (project filter not applied). Fixed by removing the `s.CloudState == nil` guard inside `BaseServer.ContainerList` so the filter runs unconditionally. Same lineage as BUG-991 / BUG-992 (handler delegation losing implicit behavior).
**Source**: Pattern surfaced during Phase 161 closeout — no external citation yet, but the underlying mechanism is well-described in [Augment Code's pattern 8](https://www.augmentcode.com/guides/debugging-ai-generated-code-8-failure-patterns-and-fixes) ("Subtle behaviour changes during refactor") and in pattern 12 here (concurrency / correctness bugs replicated across the codebase) — different shape (asymmetry, not replication), same family.

### 35. Text-rewriting scripts lose semantic information
**Description**: Sed / regex / line-by-line scripts applied to source code can break syntax in ways the linter doesn't catch and only tests surface. Common failure modes: joining adjacent lines when a trailing newline gets stripped, eating required function args when a regex matches `, ""` across nested call expressions, leaving orphaned tokens when prefix-stripping is over-eager.
**Example**: A regex `^//\s*Phase \d+ — (.*)$` strips the prefix; the rewrite forgets to re-append the trailing newline; the next line gets joined to the comment; an `s.mux.HandleFunc("GET /login/oauth/authorize", ...)` route disappears into the previous comment line.
**Why it's bad**: The damage isn't local. A single bad regex can clobber routes / handlers / arg lists across dozens of files; the build still succeeds for unrelated reasons (Go doesn't care about route registration at compile time); tests catch a tiny fraction of the surface.
**Sockerless instance**: Phase 161 P161.8 — the BUG-994 phase/BUG-ref stripping script lost trailing newlines on 3 sites in bleephub, joining route-registration lines into preceding comments and breaking 3 GraphQL routes (caught by `TestOAuthToken*`). Also the sed for `, ""` in P161.7 ate required args from `t.Setenv("FOO")`, `SimulatorEnv(CloudGCP, 5000)`, and 7+ `NewProjectManager(pm, nil)` call sites — all caught by `go test`, none by `go build`.
**Mitigation**: For bulk code rewrites prefer language-aware tools — `gofmt -r`, `goimports`, AST visitors via `go/ast`, `tree-sitter` queries. When sed / regex *must* be used, run `go build ./...` AND `go test ./...` immediately after the script — don't trust visual inspection of the diff.
**Source**: Pattern surfaced during Phase 161 closeout. Related discussion in [Christopher Montes, "Lint Against the Machine"](https://medium.com/@montes.makes/lint-against-the-machine-a-field-guide-to-catching-ai-coding-agent-anti-patterns-3c4ef7baeb9e) on the broader theme of static analysis being insufficient for AI-authored changes.

## Maintainer-stated AI-PR policies

These are the public stances from established projects on accepting AI-generated contributions:

- **Zig** — *Total ban.* Code of Conduct: *"No LLMs for issues. No LLMs for pull requests. No LLMs for comments on the bug tracker, including translation."* Cited reasons: PRs that "wouldn't even compile," "Insane 10 thousand line long first time PRs," contributors denying LLM use while submitting clearly LLM-generated text. — [byteiota.com](https://byteiota.com/zig-bans-ai-contributions-contributor-poker-philosophy/)
- **curl** — Bug-bounty program shut down end-Jan 2026 specifically because of AI-slop reports.
- **QEMU, NetBSD, Gentoo, GNOME Loupe** — Outright bans reported alongside Zig's.
- **Apache, Fedora, Linux Foundation** — Conditional acceptance with explicit disclosure required.
- **Simon Willison** — Distinguishes *vibe coding* (irresponsible) from *vibe engineering*: *"Your agent might claim something works without having actually tested it at all"* — [simonwillison.net](https://simonwillison.net/2025/Oct/7/vibe-engineering/).

## Famous vibe-coded incidents

- **PocketOS (Cursor + Claude Opus 4.6)** — Production DB + all Railway volume backups deleted in 9s.
- **Replit / SaaStr (Jason Lemkin)** — AI agent deleted 1,206 executive records and 1,196 company records under an ALL-CAPS code freeze.
- **Lovable-built apps (170 applications)** — Inverted access control across 170 production apps.
- **Moltbook** — 1.5M API keys exposed due to missing Row-Level Security.
- **Base44** — Platform-wide authentication bypass.
- **OpencodeCLI** — 1,200 open PRs, mostly AI-generated, one maintainer.

## Categories used in this doc

| Cat | Patterns | Sockerless policy |
|---|---|---|
| Fake / fallback / silent degradation | 1, 7, 9, 18 | "No fakes / no fallbacks / no silent shims" + fail-loud on persistence-open failure |
| Anemic / fake tests | 2, 3, 28, 29 | External fixtures use real client; specs are the authoritative reference, not the implementation; assert on contract substrings not implementation metadata; no mocks (sims at cloud-API fidelity) |
| Lack of real-world fidelity | 6, 21, 22, 29 | Reference-adaptor framing (Phase 157); cross-cloud sweep on every find |
| Verbose / useless comments + drift | 8, 33 | Default to no comments; only WHY-non-obvious lines allowed; per-component README contract makes doc-vs-code drift visible |
| Assumptions without research | 4, 22, 30 | Grep before adding a call; verify upstream package existence; spec-first; flag early premises that propagate (cascade audit) |
| Rush instead of planning | 19, 20, 26 | `PLAN.md` phase-by-phase; acceptance criteria; manual-test cycle; budget for re-verification |
| Myopia / context loss | 11, 12, 13, 17 | State save every task; `MEMORY.md` carries invariants; cross-cloud sweep |
| Model-specific habits | 5, 14, 24, 25 | "Right fix not quick fix"; three-lines-better-than-abstraction rule; explicit re-verification overrides first-pass sycophancy; comprehension-debt audit at phase boundaries |
| Security / supply chain | 4, 10, 15, 16, 23 | README caveat block; pre-commit hooks; user-merges-every-PR |
| Refactor / pruning hygiene | 11, 12, 14, 27, 34, 35 | Always-delete-something audit per phase; language-aware tools (gofmt, ast) over sed for code rewrites; refactor-delegation safety-net audit |
| Operational gotchas (process, not slop) | 31, 32 | Verify `git log` after every commit attempt; one explicit re-verification cycle per substantial change |

## Project-local skills

Five Claude skills under `.claude/skills/` operationalise the catalogue:

- [`avoid-vibe-slop`](../.claude/skills/avoid-vibe-slop/SKILL.md) — read before every non-trivial change. 26-item checklist references patterns by number. Phase 161 closeout expanded it from 17 items to 26 with new categories: refactor-delegation safety-net audit (Q9, pattern 34), text-rewriting language-aware tools (Q10, pattern 35), metadata-in-test-assertions audit (Q13/Q15, pattern 28), pruning audit (Q18, pattern 27), doc-sync (Q19, pattern 33), post-commit SHA verification (Q25, pattern 31), explicit re-verification pass (Q26, patterns 24+32).
- [`adaptor-fidelity-check`](../.claude/skills/adaptor-fidelity-check/SKILL.md) — used when touching backends/simulators/bleephub; verifies the change preserves the reference-adaptor contract. Phase 160 extended it with SDK-serializer-source verification (step 1a) and TF-provider `resourceXxxRead` inspection (step 1b). Pattern 29 (mock drift) is the explicit anti-pattern this skill exists to prevent.
- [`manual-test`](../.claude/skills/manual-test/SKILL.md) — runs the canonical manual smoke per component (no mocks, real adaptor). Pattern 29 mitigation.
- [`sim-handler-checklist`](../.claude/skills/sim-handler-checklist/SKILL.md) — pre-write checklist for new `simulators/<cloud>/<service>.go` files. Distilled from Phase 159 (CloudFront / ACM / Route 53 / WAFv2 / Amplify / IAM SLR/OIDC); every load-bearing fix came from one of four checks the skill enumerates.
- [`cross-resource-stack-test`](../.claude/skills/cross-resource-stack-test/SKILL.md) — codifies the `TestStackProductionShape` pattern: declare `output` blocks for every cross-resource attribute, read `terraform output -json` in Go, assert what references resolve to (not just that apply doesn't crash).

## How to extend this doc

1. **Find a new pattern.** Either internally (a fixed bug exemplifies a class) or externally (an article / HN thread / incident).
2. **Verify the source.** No source = no entry. Quote verbatim.
3. **Map to sockerless.** If we've seen it: link bug ID. If not: write "not yet observed" + the mitigation in place.
4. **Append, don't renumber.** Pattern numbers are stable references; skills cite them.
5. **Update the category table** if the pattern needs a new category.

Last updated: 2026-05-16 (Phase 161 closeout — 12 new patterns (24–35) appended from Phase 161 fix lessons + late-2025 / 2026 external research. Patterns 34 + 35 are Phase-161-native; 24–33 are cited from external sources first cataloged this round. Phase 161 sub-task mappings included on every entry that hit the same shape.)
