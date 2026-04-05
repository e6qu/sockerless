---
name: All synthetic behavior is a bug
description: Never use synthetic/fake/placeholder data in backends — always implement real behavior or file a bug
type: feedback
---

All synthetic behavior in backends is a bug, no exceptions. Fake data, placeholder stats, hardcoded values, in-memory-only state when cloud resources are available — all bugs.

**Why:** The user considers any fake behavior as technical debt that masks real issues. Synthetic behavior (e.g., fake image configs, fake IPs, fake stats) has caused real bugs in production testing (BUG-591: synthetic `/bin/sh` CMD caused nginx to exit immediately).

**How to apply:** When encountering synthetic behavior in the codebase, treat it as a bug to fix, not as intended design. When implementing new features, always use real data from the cloud provider. If real implementation is not feasible, file a bug and track it — do not silently fall back to synthetic behavior.
