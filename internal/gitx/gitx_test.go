package gitx_test

import (
	"strings"
	"testing"

	"github.com/gleno/unblamed/internal/gitx"
	"github.com/gleno/unblamed/internal/repotest"
)

func TestHeadSHAAndTreeSHA(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "one\ntwo\n"})
	if s, err := gitx.HeadSHA(r.Git); err != nil || len(s) != 40 {
		t.Fatalf("head sha = %q, err=%v", s, err)
	}
	if s, err := gitx.HeadTreeSHA(r.Git); err != nil || len(s) != 40 {
		t.Fatalf("head tree sha = %q, err=%v", s, err)
	}
}

func TestLsFilesTracked(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"a.txt":     "one\n",
		"dir/b.txt": "two\n",
	})
	files, err := gitx.LsFilesTracked(r.Git)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(files, ",")
	if got != "a.txt,dir/b.txt" {
		t.Fatalf("ls-files = %q", got)
	}
}

func TestBlame(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "l1\nl2\n"})
	r.CommitAs("Bob", "bob@example.com", map[string]string{"a.txt": "l1\nl2\nl3\n"})
	lines, err := gitx.Blame(r.Git, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %+v", len(lines), lines)
	}
	if lines[0].Email != "alice@example.com" || lines[0].Line != 1 {
		t.Errorf("line 1: got %+v", lines[0])
	}
	if lines[1].Email != "alice@example.com" || lines[1].Line != 2 {
		t.Errorf("line 2: got %+v", lines[1])
	}
	if lines[2].Email != "bob@example.com" || lines[2].Line != 3 {
		t.Errorf("line 3: got %+v", lines[2])
	}
}

func TestStatusPorcelainCleanAndDirty(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "x\n"})
	s, err := gitx.StatusPorcelain(r.Git)
	if err != nil {
		t.Fatal(err)
	}
	if s != "" {
		t.Errorf("expected clean, got %q", s)
	}
	r.WriteFiles(map[string]string{"a.txt": "y\n"})
	s, err = gitx.StatusPorcelain(r.Git)
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Errorf("expected dirty, got empty")
	}
}

func TestDiffUnifiedZero(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "one\ntwo\nthree\n"})
	tree, _ := gitx.HeadTreeSHA(r.Git)
	r.WriteFiles(map[string]string{"a.txt": "one\nTWO\nthree\n"})
	out, err := gitx.DiffUnifiedZero(r.Git, tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "-two") || !strings.Contains(string(out), "+TWO") {
		t.Fatalf("diff missing changes: %s", out)
	}
}

func TestCaptureWorkingTreeSHA(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "x\n"})
	baseline, _ := gitx.HeadTreeSHA(r.Git)
	r.WriteFiles(map[string]string{"a.txt": "y\n"})
	captured, err := gitx.CaptureWorkingTreeSHA(r.Git)
	if err != nil {
		t.Fatal(err)
	}
	if captured == baseline {
		t.Fatalf("captured tree = baseline; expected divergence")
	}
	if len(captured) != 40 {
		t.Fatalf("captured=%q", captured)
	}
}
