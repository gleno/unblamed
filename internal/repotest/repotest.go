package repotest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gleno/unblamed/internal/cli"
	"github.com/gleno/unblamed/internal/gitx"
)

type Repo struct {
	t    *testing.T
	Dir  string
	Home string
	Git  *gitx.Shell
	date time.Time
}

func New(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()

	env := baseEnv(home, "committer", "committer@example.com", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sh := gitx.NewShell(dir).WithEnv(env)

	if _, err := sh.Exec("init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if _, err := sh.Exec("config", "commit.gpgsign", "false"); err != nil {
		t.Fatalf("git config gpgsign: %v", err)
	}
	if _, err := sh.Exec("config", "user.name", "committer"); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	if _, err := sh.Exec("config", "user.email", "committer@example.com"); err != nil {
		t.Fatalf("git config user.email: %v", err)
	}

	return &Repo{
		t:    t,
		Dir:  dir,
		Home: home,
		Git:  sh,
		date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func baseEnv(home, name, email string, when time.Time) []string {
	ts := when.Format(time.RFC3339)
	return []string{
		"HOME=" + home,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name,
		"GIT_COMMITTER_EMAIL=" + email,
		"GIT_AUTHOR_DATE=" + ts,
		"GIT_COMMITTER_DATE=" + ts,
		"PATH=" + os.Getenv("PATH"),
	}
}

func (r *Repo) RunCLI(args ...string) (int, string, string) {
	r.t.Helper()
	r.date = r.date.Add(time.Second)
	env := baseEnv(r.Home, "committer", "committer@example.com", r.date)
	var stdout, stderr bytes.Buffer
	code := cli.Run(cli.Env{Dir: r.Dir, Stdout: &stdout, Stderr: &stderr, GitEnv: env}, args)
	return code, stdout.String(), stderr.String()
}

func (r *Repo) withIdentity(name, email string) *gitx.Shell {
	r.date = r.date.Add(time.Second)
	env := baseEnv(r.Home, name, email, r.date)
	return gitx.NewShell(r.Dir).WithEnv(env)
}

func (r *Repo) WriteFiles(files map[string]string) {
	r.t.Helper()
	for rel, content := range files {
		p := filepath.Join(r.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			r.t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func (r *Repo) DeleteFiles(paths ...string) {
	r.t.Helper()
	for _, rel := range paths {
		if err := os.Remove(filepath.Join(r.Dir, rel)); err != nil {
			r.t.Fatalf("delete %s: %v", rel, err)
		}
	}
}

func (r *Repo) CommitAs(name, email string, files map[string]string) string {
	r.t.Helper()
	r.WriteFiles(files)
	sh := r.withIdentity(name, email)
	if _, err := sh.Exec("add", "-A"); err != nil {
		r.t.Fatalf("git add: %v", err)
	}
	if _, err := sh.Exec("commit", "-q", "-m", "commit by "+name); err != nil {
		r.t.Fatalf("git commit: %v", err)
	}
	sha, err := gitx.HeadSHA(sh)
	if err != nil {
		r.t.Fatalf("rev-parse HEAD: %v", err)
	}
	return sha
}

func (r *Repo) Read(rel string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Dir, rel))
	if err != nil {
		r.t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func (r *Repo) BlameAuthorEmail(path string, line int) string {
	r.t.Helper()
	blame, err := gitx.Blame(r.Git, path)
	if err != nil {
		r.t.Fatalf("blame %s: %v", path, err)
	}
	for _, b := range blame {
		if b.Line == line {
			return b.Email
		}
	}
	r.t.Fatalf("no blame entry for %s:%d", path, line)
	return ""
}

func (r *Repo) CommitEmails(n int) []string {
	r.t.Helper()
	out, err := r.Git.Exec("log", "--format=%ae", fmt.Sprintf("-%d", n))
	if err != nil {
		r.t.Fatalf("git log: %v", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func (r *Repo) HeadSHA() string {
	r.t.Helper()
	s, err := gitx.HeadSHA(r.Git)
	if err != nil {
		r.t.Fatalf("head sha: %v", err)
	}
	return s
}

func (r *Repo) HeadTreeSHA() string {
	r.t.Helper()
	s, err := gitx.HeadTreeSHA(r.Git)
	if err != nil {
		r.t.Fatalf("head tree sha: %v", err)
	}
	return s
}

func (r *Repo) Snapshot() map[string]string {
	r.t.Helper()
	out, err := r.Git.Exec("ls-files", "-z")
	if err != nil {
		r.t.Fatalf("ls-files: %v", err)
	}
	raw := bytes.TrimRight(out, "\x00")
	files := map[string]string{}
	if len(raw) == 0 {
		return files
	}
	for _, p := range bytes.Split(raw, []byte{0}) {
		rel := string(p)
		b, err := os.ReadFile(filepath.Join(r.Dir, rel))
		if err != nil {
			r.t.Fatalf("read %s: %v", rel, err)
		}
		files[rel] = string(b)
	}
	return files
}

func ExecGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func SortedKeys[M ~map[string]V, V any](m M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
