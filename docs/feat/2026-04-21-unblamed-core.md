# unblamed — author-preserving reformat tool

## Problem

Running a formatter (prettier, black, rustfmt, gofmt, ...) across a repo in a single commit destroys `git blame` — every line's last-touched author becomes whoever ran the formatter. This hides real authorship and makes blame useless for the reformatted files.

## Proposed solution

A tool that splits a bulk reformat into one commit per original author, so that `git blame` on the reformatted tree still points at the person who actually wrote each line.

### Algorithm

1. On a clean branch, run `git blame --line-porcelain` on every file to build a map: `file:line → author`.
2. Run the formatter on the working tree. Don't commit yet.
3. For each line in the reformatted files, determine which original line it corresponds to so the author can be looked up. This is the hard part — formatters rewrite whitespace, wrap lines, collapse/expand blocks. A diff algorithm (e.g. `git diff --word-diff`) or a custom hunk-walker should be able to map new lines back to old lines well enough.
4. For each author, construct a tree where only their lines have been reformatted and everyone else's lines are untouched, then commit it with `git commit-tree` (or stage a patch with `git apply` + `git commit --author=...`).
5. Repeat per author until the final tree matches the fully-reformatted tree.

## UX

Two-step CLI wrapped around the user's own formatter invocation:

```
unblamed stage
  # <- user runs their formatter script here ->
unblamed apply
```

- `unblamed stage` — snapshot the pre-format state: per-file, per-line author map from `git blame`, plus the original tree hash. Persist to `.git/unblamed/` (or similar) so `apply` can pick it up.
- `unblamed apply` — diff the working tree against the staged snapshot, partition changes by original author, and emit one commit per author (ordered deterministically — e.g. by author name, or by first-touched line). The final commit's tree must exactly match the fully-reformatted working tree.

## Open questions / to resolve during brainstorm

- Line-mapping strategy when formatters wrap/join lines: per-character attribution? majority-author-wins per new line?
- What to do when a new line has no clear ancestor (e.g. formatter-inserted blank line, reflowed import block)? Attribute to the most recent nearby author? The committer running the tool?
- Commit author identity: use the original author's name+email from blame, or require a configured mapping?
- Ordering of per-author commits — does it matter for the final tree? (It shouldn't, but intermediate trees must be valid/compilable? Probably not a goal v1.)
- Scope: single formatter run vs. arbitrary refactor? v1 = formatter-only (whitespace / reflow), not semantic edits.
- Binary files, deletes, renames — ignore or pass through untouched?
- Interaction with existing staged changes when `unblamed stage` runs — require clean tree?
