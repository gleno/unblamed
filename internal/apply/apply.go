package apply

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gleno/unblamed/internal/attribute"
	"github.com/gleno/unblamed/internal/gitx"
	"github.com/gleno/unblamed/internal/snapshot"
)

type Result struct {
	Commits      []string
	Plan         attribute.Plan
	DryRun       bool
	Authors      []attribute.AuthorKey
	TargetTree   string
	RollbackSHA  string
}

type Options struct {
	DryRun bool
	Stderr io.Writer
}

func Run(g gitx.Git, opts Options) (*Result, error) {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	snap, err := snapshot.Load(g)
	if err != nil {
		return nil, fmt.Errorf("no staged snapshot: run `unblamed stage` first")
	}
	headSHA, err := gitx.HeadSHA(g)
	if err != nil {
		return nil, err
	}
	if headSHA != snap.HeadSHA {
		return nil, fmt.Errorf("HEAD moved since stage (snapshot=%s, HEAD=%s); run `unblamed abort` or reset", short(snap.HeadSHA), short(headSHA))
	}
	targetTree, err := gitx.CaptureWorkingTreeSHA(g)
	if err != nil {
		return nil, fmt.Errorf("capture working tree: %w", err)
	}

	diffRaw, err := g.Exec("diff", "--no-color", "--unified=0", "--no-renames", snap.TreeSHA, targetTree)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	files, err := attribute.ParseDiff(diffRaw)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	fallback := snapshot.Identity{Name: snap.Fallback.Name, Email: snap.Fallback.Email}
	plan := attribute.BuildPlan(snap, files, fallback)

	res := &Result{
		Plan:        plan,
		DryRun:      opts.DryRun,
		Authors:     plan.Order,
		TargetTree:  targetTree,
		RollbackSHA: headSHA,
	}

	if opts.DryRun {
		return res, nil
	}

	if len(plan.Order) == 0 {
		if err := snapshot.Delete(g); err != nil {
			return nil, err
		}
		return res, nil
	}

	if _, err := g.Exec("clean", "-fd"); err != nil {
		return nil, fmt.Errorf("clean untracked: %w", err)
	}
	if err := gitx.ResetHard(g, headSHA); err != nil {
		return nil, fmt.Errorf("reset to pre-format state: %w", err)
	}

	for i, author := range plan.Order {
		if err := materializeAuthor(g, snap, &plan, i, targetTree); err != nil {
			rollback(g, headSHA)
			return nil, fmt.Errorf("materialize %s: %w", author.Email, err)
		}
		sha, err := gitx.CommitWithAuthor(g, author.Name, author.Email, commitMessage(author))
		if err != nil {
			rollback(g, headSHA)
			return nil, fmt.Errorf("commit %s: %w", author.Email, err)
		}
		res.Commits = append(res.Commits, sha)
	}

	finalTree, err := gitx.HeadTreeSHA(g)
	if err != nil {
		rollback(g, headSHA)
		return nil, fmt.Errorf("rev-parse final tree: %w", err)
	}
	if finalTree != targetTree {
		rollback(g, headSHA)
		return nil, fmt.Errorf("final tree %s does not match reformatted tree %s; rolled back", short(finalTree), short(targetTree))
	}

	if err := snapshot.Delete(g); err != nil {
		return nil, fmt.Errorf("delete snapshot: %w", err)
	}
	return res, nil
}

func materializeAuthor(g gitx.Git, snap *snapshot.Snapshot, plan *attribute.Plan, idx int, targetTree string) error {
	if err := gitx.ResetHard(g, "HEAD"); err != nil {
		return err
	}
	touched := plan.FilesTouchedCumulative(idx)
	fileOps := map[string]attribute.FileOp{}
	for _, op := range plan.FileOpsUpTo(idx) {
		fileOps[op.Path] = op
	}

	author := plan.Order[idx]
	thisHunks := plan.Hunks[author]
	thisOps := plan.FileOps[author]

	filesThisAuthor := map[string]struct{}{}
	for _, h := range thisHunks {
		filesThisAuthor[h.File] = struct{}{}
	}
	for _, op := range thisOps {
		filesThisAuthor[op.Path] = struct{}{}
		if op.OldPath != "" {
			filesThisAuthor[op.OldPath] = struct{}{}
		}
	}

	for file := range filesThisAuthor {
		if op, isOp := fileOps[file]; isOp {
			if err := applyFileOp(g, op, targetTree); err != nil {
				return err
			}
			continue
		}
		hunks := plan.HunksForFileUpTo(file, idx)
		oldContent, err := readBlob(g, snap.TreeSHA, file)
		if err != nil {
			return fmt.Errorf("read old %s: %w", file, err)
		}
		newContent := attribute.ApplyHunks(oldContent, hunks)
		abs := filepath.Join(g.Dir(), file)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(newContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
	}

	_ = touched
	if _, err := g.Exec("add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	return nil
}

func applyFileOp(g gitx.Git, op attribute.FileOp, targetTree string) error {
	abs := filepath.Join(g.Dir(), op.Path)
	switch op.Kind {
	case attribute.FileDeleted:
		_ = os.Remove(abs)
	case attribute.FileAdded, attribute.FileBinary, attribute.FileModified:
		content, err := g.Exec("show", targetTree+":"+op.Path)
		if err != nil {
			return fmt.Errorf("read target %s: %w", op.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readBlob(g gitx.Git, treeSHA, path string) (string, error) {
	out, err := g.Exec("show", treeSHA+":"+path)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func rollback(g gitx.Git, head string) {
	_ = gitx.ResetHard(g, head)
}

func commitMessage(a attribute.AuthorKey) string {
	return "unblamed: reformat (" + a.Name + ")"
}

func short(s string) string {
	if len(s) >= 7 {
		return s[:7]
	}
	return s
}

