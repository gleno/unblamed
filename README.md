# unblamed

A bulk formatter commit (prettier, gofmt, black, ...) destroys `git blame`: every line's last-touched author becomes whoever ran the formatter. `unblamed` splits that single reformat into one commit per original author, so `git blame` on the reformatted tree still points at the people who actually wrote the code.

## Install

```
go install github.com/gleno/unblamed/cmd/unblamed@latest
```

Requires Go 1.26+ and a `git` binary on `$PATH`.

## Usage

```
# 1. Make sure the working tree is clean.
# 2. Snapshot per-line authorship of every tracked file.
unblamed stage

# 3. Run whatever formatter you want. Don't commit.
gofmt -w ./...

# 4. Split the pending changes into per-author commits.
unblamed apply
```

Result: one commit per original author, authored by them, committed by you.

### Other subcommands

```
unblamed status        # is there an active snapshot?
unblamed abort         # discard the snapshot, no tree changes
unblamed apply --dry-run  # print the plan, make no commits
```

## How attribution works (v1)

Each hunk in `git diff pre-format post-format` is assigned to one author:

- If the hunk removes lines, the owner is the author with the most characters among the removed lines. Ties go to the author who appears first in the hunk.
- If the hunk is pure insertion, the owner is the author of the line immediately before it.
- Brand-new files, deletions, and binary changes are attributed to the fallback author (your `git config user.email`).

Authors are ordered alphabetically by email; the fallback author (if present) always goes last. At the end, `unblamed` verifies that the final tree equals the fully-reformatted tree and rolls back if anything went wrong.

## Limitations

- Single formatter run per `stage`/`apply` cycle.
- No rename tracking (renames look like delete + add, both attributed to the fallback author).
- macOS / Linux only in v1.
- Per-hunk attribution can misattribute a joined line to the author with more original characters. For typical whitespace-and-reflow formatting this is fine; for aggressive transformations, consider splitting your formatter into multiple passes.

## License

MIT.
