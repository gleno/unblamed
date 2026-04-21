package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gleno/unblamed/internal/apply"
	"github.com/gleno/unblamed/internal/gitx"
	"github.com/gleno/unblamed/internal/snapshot"
)

type Env struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
	GitEnv []string
}

func (e Env) newGit() *gitx.Shell {
	g := gitx.NewShell(e.Dir)
	if len(e.GitEnv) > 0 {
		g = g.WithEnv(e.GitEnv)
	}
	return g
}

func Run(env Env, args []string) int {
	if env.Stdout == nil {
		env.Stdout = os.Stdout
	}
	if env.Stderr == nil {
		env.Stderr = os.Stderr
	}
	if len(args) == 0 {
		printUsage(env.Stdout)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "stage":
		return runStage(env, rest)
	case "apply":
		return runApply(env, rest)
	case "status":
		return runStatus(env, rest)
	case "abort":
		return runAbort(env, rest)
	case "-h", "--help", "help":
		printUsage(env.Stdout)
		return 0
	default:
		fmt.Fprintf(env.Stderr, "unknown command: %s\n", cmd)
		printUsage(env.Stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: unblamed <command>

commands:
  stage     snapshot blame + HEAD tree before you run the formatter
  apply     split uncommitted formatter changes into per-author commits
  status    show whether a snapshot exists
  abort     discard the current snapshot (no tree changes)

options:
  apply --dry-run   print the commit plan without committing`)
}

func runStage(env Env, _ []string) int {
	g := env.newGit()
	status, err := gitx.StatusPorcelain(g)
	if err != nil {
		return errExit(env, err, 2)
	}
	if strings.TrimSpace(status) != "" {
		fmt.Fprintln(env.Stderr, "working tree is dirty; commit or stash before `unblamed stage`")
		return 2
	}
	if snapshot.Exists(g) {
		fmt.Fprintln(env.Stderr, "snapshot already exists; run `unblamed abort` first")
		return 2
	}
	headSHA, err := gitx.HeadSHA(g)
	if err != nil {
		return errExit(env, err, 2)
	}
	treeSHA, err := gitx.HeadTreeSHA(g)
	if err != nil {
		return errExit(env, err, 2)
	}
	files, err := gitx.LsFilesTracked(g)
	if err != nil {
		return errExit(env, err, 2)
	}
	fallback, err := snapshot.FallbackFromConfig(g)
	if err != nil {
		return errExit(env, err, 2)
	}
	s := &snapshot.Snapshot{
		HeadSHA:   headSHA,
		TreeSHA:   treeSHA,
		Files:     map[string][]snapshot.LineAuthor{},
		Fallback:  snapshot.Identity{Name: fallback.Name, Email: fallback.Email},
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, f := range files {
		isBin, err := gitx.IsBinaryAtHead(g, f)
		if err != nil {
			return errExit(env, fmt.Errorf("probe %s: %w", f, err), 2)
		}
		if isBin {
			continue
		}
		lines, err := gitx.Blame(g, f)
		if err != nil {
			return errExit(env, fmt.Errorf("blame %s: %w", f, err), 2)
		}
		if len(lines) == 0 {
			continue
		}
		entries := make([]snapshot.LineAuthor, 0, len(lines))
		for _, ln := range lines {
			entries = append(entries, snapshot.LineAuthor{Line: ln.Line, Author: ln.Author, Email: ln.Email})
		}
		s.Files[f] = entries
	}
	if err := snapshot.Save(g, s); err != nil {
		return errExit(env, err, 2)
	}
	fmt.Fprintf(env.Stdout, "staged %d files at %s\n", len(s.Files), short(headSHA))
	return 0
}

func runApply(env Env, args []string) int {
	dryRun := false
	for _, a := range args {
		switch a {
		case "--dry-run", "-n":
			dryRun = true
		default:
			fmt.Fprintf(env.Stderr, "unknown apply flag: %s\n", a)
			return 2
		}
	}
	g := env.newGit()
	res, err := apply.Run(g, apply.Options{DryRun: dryRun, Stderr: env.Stderr})
	if err != nil {
		if isUserError(err) {
			fmt.Fprintln(env.Stderr, err)
			return 2
		}
		fmt.Fprintln(env.Stderr, err)
		return 3
	}
	if dryRun {
		printPlan(env.Stdout, res)
		return 0
	}
	for i, author := range res.Authors {
		fmt.Fprintf(env.Stdout, "%s %s <%s>\n", short(res.Commits[i]), author.Name, author.Email)
	}
	return 0
}

func runStatus(env Env, _ []string) int {
	g := env.newGit()
	if !snapshot.Exists(g) {
		fmt.Fprintln(env.Stdout, "no snapshot")
		return 0
	}
	s, err := snapshot.Load(g)
	if err != nil {
		return errExit(env, err, 2)
	}
	fmt.Fprintf(env.Stdout, "snapshot at %s (%d files), created %s\n", short(s.HeadSHA), len(s.Files), s.CreatedAt)
	return 0
}

func runAbort(env Env, _ []string) int {
	g := env.newGit()
	if !snapshot.Exists(g) {
		fmt.Fprintln(env.Stdout, "no snapshot to abort")
		return 0
	}
	if err := snapshot.Delete(g); err != nil {
		return errExit(env, err, 2)
	}
	fmt.Fprintln(env.Stdout, "snapshot discarded")
	return 0
}

func printPlan(w io.Writer, res *apply.Result) {
	for _, author := range res.Authors {
		hunks := res.Plan.Hunks[author]
		ops := res.Plan.FileOps[author]
		fmt.Fprintf(w, "%s <%s>: %d hunks, %d file-ops\n", author.Name, author.Email, len(hunks), len(ops))
	}
}

func errExit(env Env, err error, code int) int {
	fmt.Fprintln(env.Stderr, err)
	return code
}

func isUserError(err error) bool {
	msg := err.Error()
	for _, needle := range []string{
		"no staged snapshot",
		"HEAD moved",
		"working tree is dirty",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func short(s string) string {
	if len(s) >= 7 {
		return s[:7]
	}
	return s
}

