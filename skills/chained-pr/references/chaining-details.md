# Chained PR Details

## Host Detection & Provider Routing

Before creating branches or PRs, detect the Git host using read-only inspection:

```bash
git remote get-url origin
```

- **GitHub (`github.com` or GitHub Enterprise)**: Proceed to GitHub Provider Route.
- **Non-GitHub (`gitlab.com`, `bitbucket.org`, `gitea`, self-hosted GitLab/Gitea, etc.)**: Proceed to Portable Provider Routes.
- **Unknown or Ambiguous (no remote, unresolvable URL)**: **Fail-closed stop**. Do not guess or create branches until the remote host is confirmed.

---

## GitHub Provider Route (Native Stack)

When targeting GitHub and native stack capability is proven:

### Capability Gate
- **Proven Capability**: `gh-stack` (or equivalent GitHub stack extension/tool) is installed, authenticated, and capable.
- **Unproven Capability**: If `gh-stack` is missing or unverified, **fail-closed stop** with guidance:
  ```text
  GitHub stack capability is unproven. Please install and authorize gh-stack:
    gh extension install graphite-dev/gh-stack  # or approved stack extension
  Do not silently fall back to portable branch choreography on GitHub without user confirmation.
  ```

### Pinned `gh-stack` Command Surface
```bash
# Initialize / submit stack
gh stack branch create <branch-name>
gh stack submit --title "feat(scope): slice title" --body-file pr-body.md

# Sync and restack
gh stack sync
gh stack restack
```

### Review Budget & Issue Linking
- Keep each stacked PR within **≤400 changed lines** (`additions + deletions`).
- Use `Closes #N` when a PR completely resolves an issue; use `Refs #N` for intermediate slices.
- **Remote Authority Boundaries**: Respect remote branch protection rules; do not force-push to trunk or override protected integration branches.

---

## Non-GitHub Provider Routes (Portable)

For positively identified non-GitHub hosts (GitLab, Bitbucket, Gitea, etc.), use portable branch choreography.

### Strategy Comparison

| | Stacked PRs to main | Feature Branch Chain |
|---|---|---|
| Speed | Each slice can ship in order | Full feature waits for tracker merge |
| Rollback | Revert individual main PRs | Revert/hold the whole feature branch |
| Risk | Partial behavior may land | Nothing lands until the chain completes |
| Complexity | Simpler retarget/rebase flow | Requires tracker and strict diff hygiene |

### Feature Branch Chain
Use when the feature branch accumulates the final integration while child PRs are reviewed as focused slices.

```text
main
 └── feat/my-feature              ← tracker/final integration branch
      ↑ PR #1 base: feat/my-feature
      └── feat/my-feature-01-core
           ↑ PR #2 base: feat/my-feature-01-core
           └── feat/my-feature-02-shared
                ↑ PR #3 base: feat/my-feature-02-shared
                └── feat/my-feature-03-slice
```

1. Create the feature/tracker branch from `main`.
2. Open the tracker PR to `main`; mark it draft/no-merge.
3. Create PR #1 from a child branch and target it to the tracker branch.
4. Create each later child branch from the previous PR branch and target it to that parent branch.
5. Merge/integrate children in order; merge the tracker only after the chain is complete.

### Stacked PRs to Main
Use when each slice can land on `main` in order.

```text
main <- PR 1: foundation
          └── PR 2: feature slice built on PR 1
                └── PR 3: docs/tests built on PR 2
```

---

## Chain Closing, Merging, and Retargeting Safety

When merging or closing PRs in a chain (both portable and native workflows):

### Pre-Deletion Safety Check
Before deleting any merged parent branch, check for open dependent PRs targeting that branch:

```bash
gh pr list --base <branch-to-delete> --state open
```

### Retargeting Before Branch Deletion
1. If open child PRs exist, **retarget them before deleting the parent branch**:
   ```bash
   gh pr edit <CHILD_PR_NUMBER> --base <new-base-branch>
   ```
2. Once child PRs are retargeted to the new base branch (or `main`), safely delete the merged parent branch.

### Recovery from Premature Branch Deletion
If a parent branch was deleted while child PRs were still targeting it:
1. Re-open or inspect the child PR.
2. Retarget the child PR to the current target branch (`main` or tracker).
3. Rebase child branch locally to remove duplicate merged commits:
   ```bash
   git fetch origin
   git rebase --onto origin/<new-base-branch> <deleted-parent-sha-or-tag> <child-branch>
   git push --force-with-lease origin <child-branch>
   ```

---

## Governance Isolation

- **Repo Agnostic**: This skill provides general chaining mechanisms. It must NOT enforce Gentle AI repository-specific conventions (e.g. `status:approved` issue label, internal Gentle AI workflows, or repository-specific linters) onto user repositories.
- **Respect Target Repo**: Adopt target repository PR templates, label conventions, issue tracker syntaxes, and CI policies.

---

## Chain Context Section

Append this section to the target repo's PR template; do not replace required issue/checklist sections.

```markdown
## Chain Context

| Field | Value |
|-------|-------|
| Chain | <feature or stack name> |
| Tracker PR | <#NNN or "Not needed"> |
| Position | <N of total> |
| Base | `<target branch>` |
| Depends on | <PR/issue/link or "None"> |
| Follow-up | <next PR or "None"> |
| Review budget | <changed lines> / 400 |
| Starts at | <branch, PR, or state this builds on> |
| Ends with | <standalone result delivered by this PR> |

### Chain Overview

```text
main
 └── #NNN Previous PR
      └── 📍 #NNN This PR
           └── #NNN Next PR
```

### Scope
- Includes: <focused unit>
- Excludes: <deferred work>

### Autonomy
- [ ] CI is expected to pass for this PR branch
- [ ] This PR has one deliverable scope
- [ ] This PR can be rolled back without unrelated changes
- [ ] Tests, docs, or manual verification cover this unit
```

---

## Commands

```bash
# Host detection (read-only)
git remote get-url origin

# Inspect PR diff size
gh pr view <PR_NUMBER> --json additions,deletions,changedFiles,title,url

# Portable PR creation
gh pr create --base feat/my-feature --title "feat(scope): focused slice" --body-file pr-body.md
gh pr create --base feat/my-feature-01-core --title "feat(scope): next focused slice" --body-file pr-body.md

# Dependent PR check & retarget
gh pr list --base feat/my-feature-01-core --state open
gh pr edit <PR_NUMBER> --base feat/my-feature
```

---

## Reviewer Guidance

- Ask for a split when a PR exceeds 400 changed lines without `size:exception`.
- Route to GitHub native stack (`gh-stack`) when on GitHub with proven capability.
- Route to Feature Branch Chain or Stacked PRs for non-GitHub hosts or when using portable choreography.
- Verify child PRs against immediate parent branches; a polluted diff is a branching bug.
- Ensure child PRs are retargeted before deleting merged parent branches.
