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
**Why it's bad**: Volunteer maintainers burn out triaging hallucinations.
**Sockerless instance**: Not a public open-source target yet, so not observed. If sockerless becomes public the policy will mirror Stenberg's: AI-assisted reports OK; AI-generated reports without verified reproduction rejected.
**Source**: [bleepingcomputer.com](https://www.bleepingcomputer.com/news/security/curl-ending-bug-bounty-program-after-flood-of-ai-slop-reports/) — Stenberg: *"The main goal with shutting down the bounty is to remove the incentive for people to submit crap and non-well researched reports to us. AI generated or not."*

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
| Anemic / fake tests | 2, 3 | External fixtures use real client; specs are the authoritative reference, not the implementation |
| Lack of real-world fidelity | 6, 21, 22 | Reference-adaptor framing (Phase 157); cross-cloud sweep on every find |
| Verbose / useless comments | 8 | Default to no comments; only WHY-non-obvious lines allowed |
| Assumptions without research | 4, 22 | Grep before adding a call; verify upstream package existence; spec-first |
| Rush instead of planning | 19, 20 | `PLAN.md` phase-by-phase; acceptance criteria; manual-test cycle |
| Myopia / context loss | 11, 12, 13, 17 | State save every task; `MEMORY.md` carries invariants; cross-cloud sweep |
| Model-specific habits | 5, 14 | "Right fix not quick fix"; three-lines-better-than-abstraction rule |
| Security / supply chain | 4, 10, 15, 16 | README caveat block; pre-commit hooks; user-merges-every-PR |

## Project-local skills

Three Claude skills under `.claude/skills/` operationalise the catalogue:

- [`avoid-vibe-slop`](../.claude/skills/avoid-vibe-slop/SKILL.md) — read before every non-trivial change. Checklist references patterns by number.
- [`adaptor-fidelity-check`](../.claude/skills/adaptor-fidelity-check/SKILL.md) — used when touching backends/simulators/bleephub; verifies the change preserves the reference-adaptor contract.
- [`manual-test`](../.claude/skills/manual-test/SKILL.md) — runs the canonical manual smoke per component (no mocks, real adaptor).

## How to extend this doc

1. **Find a new pattern.** Either internally (a fixed bug exemplifies a class) or externally (an article / HN thread / incident).
2. **Verify the source.** No source = no entry. Quote verbatim.
3. **Map to sockerless.** If we've seen it: link bug ID. If not: write "not yet observed" + the mitigation in place.
4. **Append, don't renumber.** Pattern numbers are stable references; skills cite them.
5. **Update the category table** if the pattern needs a new category.

Last updated: 2026-05-13 (Phase 158 — initial catalogue).
