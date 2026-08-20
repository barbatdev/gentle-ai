---
name: chained-pr
description: "Trigger: PRs over 400 lines, stacked PRs, review slices. Split oversized changes into chained PRs that protect review focus."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "2.0"
---

## Activation Contract

Load this skill when a planned PR may exceed **400 changed lines**, SDD forecasts `400-line budget risk: High` or `Chained PRs recommended: Yes`, or the user asks for chained/stacked PRs, review slices, or reviewer-load control.

## Hard Rules

- Split PRs over **400 changed lines** unless a maintainer explicitly accepts `size:exception`.
- Keep each PR reviewable in about **≤60 minutes**.
- **Host Detection**: Detect Git host via read-only remote check. If host is unknown or ambiguous, **fail-closed stop** and ask for clarification.
- **GitHub Provider Route**:
  - If native stack capability is proven (`gh-stack`), use the GitHub native stack workflow with pinned `gh-stack` commands, review budget (≤400 lines), issue linking (`Closes #N` / `Refs #N`), and remote authority boundaries.
  - If capability is unproven, **fail-closed stop** with actionable guidance to authorize/prove capability; never silently fall back to portable choreography.
- **Non-GitHub Provider Route**:
  - Positively identified non-GitHub hosts (GitLab, Bitbucket, Gitea, etc.): use portable routes (Stacked PRs to main or Feature Branch Chain with tracker).
  - Check open dependent PRs (`gh pr list --base <branch-to-delete>` or host equivalent) and retarget child PRs before branch deletion; recover orphaned children if parent was deleted.
- **Governance Isolation**: Do not leak Gentle AI repo-specific governance (internal labels, checklists, or repo URLs) into user repositories.
- Use one deliverable work unit per PR; keep tests/docs with the unit they verify.
- Every child PR must include a dependency diagram marking the current PR with `📍`.
- Treat polluted diffs as base bugs: retarget or rebase until only the current work unit appears.
- Do not mix chain strategies after the user chooses one.

## Decision Gates

| Condition | Action |
|---|---|
| Host unknown or ambiguous | Fail-closed stop; request host clarification. |
| GitHub host + proven `gh-stack` capability | Use GitHub native stack workflow with pinned `gh-stack` commands. |
| GitHub host + unproven stack capability | Fail-closed stop; guide user to install/authorize `gh-stack`. |
| Positively identified non-GitHub host | Use portable route (Stacked PRs or Feature Branch Chain). |
| PR ≤400 changed lines and focused | Keep single PR. |
| Generated/vendor/migration diff cannot split cleanly | Ask maintainer for `size:exception`. |
| SDD provides `delivery_strategy` | Follow it before apply/PR creation. |

## Execution Steps

1. **Detect Git Host (Read-Only)**: Inspect remote URL (`git remote get-url origin`). Stop fail-closed if ambiguous or unknown.
2. **Route by Provider**:
   - If GitHub: verify `gh-stack` capability; stop fail-closed if unproven.
   - If non-GitHub (GitLab, Bitbucket, Gitea, etc.): select portable route (Stacked PRs or Feature Branch Chain).
3. **Partition Work Units**: Split changes into autonomous slices under 400 changed lines.
4. **Create Branches & PRs**:
   - Follow chosen provider route.
   - Add Chain Context to PR body without replacing target repo's PR template.
   - Link issues accurately (`Closes #N` or `Refs #N`).
5. **Verify Slices**: Verify each PR independently (CI/tests/docs, clean review diff, rollback scope).
6. **Retargeting & Deletion Safety**: Check dependent PRs before branch deletion; retarget children first; recover if deleted.

## Output Contract

Return detected host/provider, chosen strategy/route, PR order, current PR boundary, dependency diagram, review budget (`additions + deletions`), verification plan, and any `size:exception` rationale.

## References

- [references/chaining-details.md](references/chaining-details.md) — provider routes, `gh-stack` command surface, portable workflows, retargeting safety, and reviewer guidance.
