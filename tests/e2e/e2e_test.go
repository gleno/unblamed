package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gleno/unblamed/internal/repotest"
)

func TestSingleAuthor(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"a.txt": "one\ntwo\nthree\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{"a.txt": "ONE\nTWO\nTHREE\n"})
	mustRunCLI(t, r, 0, "apply")

	emails := r.CommitEmails(5)
	if len(emails) < 2 || emails[0] != "alice@example.com" {
		t.Fatalf("expected alice as latest commit's author-email, got %v", emails)
	}
	if r.Read("a.txt") != "ONE\nTWO\nTHREE\n" {
		t.Fatalf("working tree not at reformatted state: %q", r.Read("a.txt"))
	}
	for i := 1; i <= 3; i++ {
		if got := r.BlameAuthorEmail("a.txt", i); got != "alice@example.com" {
			t.Errorf("line %d blame = %s, want alice@example.com", i, got)
		}
	}
}

func TestTwoAuthorSplit(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"file.txt": "alice-one\nalice-two\n",
	})
	r.CommitAs("Bob", "bob@example.com", map[string]string{
		"file.txt": "alice-one\nalice-two\nseparator stays\nbob-three\nbob-four\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"file.txt": "ALICE-ONE\nALICE-TWO\nseparator stays\nBOB-THREE\nBOB-FOUR\n",
	})
	mustRunCLI(t, r, 0, "apply")

	if got := r.BlameAuthorEmail("file.txt", 1); got != "alice@example.com" {
		t.Errorf("line 1 -> %s", got)
	}
	if got := r.BlameAuthorEmail("file.txt", 2); got != "alice@example.com" {
		t.Errorf("line 2 -> %s", got)
	}
	if got := r.BlameAuthorEmail("file.txt", 3); got != "bob@example.com" {
		t.Errorf("separator line (bob added) -> %s", got)
	}
	if got := r.BlameAuthorEmail("file.txt", 4); got != "bob@example.com" {
		t.Errorf("line 4 -> %s", got)
	}
	if got := r.BlameAuthorEmail("file.txt", 5); got != "bob@example.com" {
		t.Errorf("line 5 -> %s", got)
	}
}

func TestThreeAuthorInterleaved(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"x.go": "package x\n\nfunc Alice() int { return 1 }\n// sep after alice\n",
	})
	r.CommitAs("Bob", "bob@example.com", map[string]string{
		"x.go": "package x\n\nfunc Alice() int { return 1 }\n// sep after alice\nfunc Bob() int { return 2 }\n// sep after bob\n",
	})
	r.CommitAs("Carol", "carol@example.com", map[string]string{
		"x.go": "package x\n\nfunc Alice() int { return 1 }\n// sep after alice\nfunc Bob() int { return 2 }\n// sep after bob\nfunc Carol() int { return 3 }\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"x.go": "package x\n\nfunc Alice() int  { return 1 }\n// sep after alice\nfunc Bob() int    { return 2 }\n// sep after bob\nfunc Carol() int  { return 3 }\n",
	})
	mustRunCLI(t, r, 0, "apply")

	cases := map[int]string{
		3: "alice@example.com",
		5: "bob@example.com",
		7: "carol@example.com",
	}
	for line, want := range cases {
		if got := r.BlameAuthorEmail("x.go", line); got != want {
			t.Errorf("line %d blame = %s, want %s", line, got, want)
		}
	}
}

func TestLineJoin(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"file.txt": "aaaaaaaaaa\n",
	})
	r.CommitAs("Bob", "bob@example.com", map[string]string{
		"file.txt": "aaaaaaaaaa\nbb\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"file.txt": "aaaaaaaaaa bb\n",
	})
	mustRunCLI(t, r, 0, "apply")

	if got := r.BlameAuthorEmail("file.txt", 1); got != "alice@example.com" {
		t.Errorf("joined line blame = %s, want alice (majority chars)", got)
	}
}

func TestLineSplit(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"file.txt": "first second third\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"file.txt": "first\nsecond\nthird\n",
	})
	mustRunCLI(t, r, 0, "apply")
	for i := 1; i <= 3; i++ {
		if got := r.BlameAuthorEmail("file.txt", i); got != "alice@example.com" {
			t.Errorf("line %d = %s, want alice", i, got)
		}
	}
}

func TestPureInsertion(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"file.txt": "one\ntwo\n",
	})
	r.CommitAs("Bob", "bob@example.com", map[string]string{
		"file.txt": "one\ntwo\nthree\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"file.txt": "one\ntwo\n\nthree\n",
	})
	mustRunCLI(t, r, 0, "apply")
	if got := r.BlameAuthorEmail("file.txt", 3); got != "alice@example.com" {
		t.Errorf("inserted blank line blame = %s, want alice (nearest above)", got)
	}
}

func TestNewFileFromFormatter(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"a.txt": "alice\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"a.txt":   "ALICE\n",
		"new.txt": "brand new file\n",
	})
	mustRunCLI(t, r, 0, "apply")
	if got := r.BlameAuthorEmail("new.txt", 1); got != "committer@example.com" {
		t.Errorf("new file line 1 = %s, want committer (fallback)", got)
	}
}

func TestDeletedFile(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"keep.txt": "keeper\n",
		"bye.txt":  "going away\n",
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{"keep.txt": "KEEPER\n"})
	r.DeleteFiles("bye.txt")
	mustRunCLI(t, r, 0, "apply")
	if _, err := os.Stat(filepath.Join(r.Dir, "bye.txt")); !os.IsNotExist(err) {
		t.Fatalf("bye.txt should be gone, stat err=%v", err)
	}
}

func TestBinaryFileChanged(t *testing.T) {
	r := repotest.New(t)
	binBefore := []byte{0x00, 0x01, 0x02, 0x03, 0x00}
	binAfter := []byte{0x00, 0xff, 0xee, 0xdd, 0x00}
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"text.txt": "alice\n",
		"blob.bin": string(binBefore),
	})
	mustRunCLI(t, r, 0, "stage")
	r.WriteFiles(map[string]string{
		"text.txt": "ALICE\n",
		"blob.bin": string(binAfter),
	})
	mustRunCLI(t, r, 0, "apply")
	got, err := os.ReadFile(filepath.Join(r.Dir, "blob.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binAfter) {
		t.Fatalf("binary content mismatch")
	}
}

func TestDirtyTreeAtStage(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "x\n"})
	r.WriteFiles(map[string]string{"a.txt": "y\n"})
	code, _, stderr := r.RunCLI("stage")
	if code != 2 {
		t.Fatalf("expected exit 2 for dirty tree, got %d (stderr=%s)", code, stderr)
	}
	if !strings.Contains(stderr, "dirty") {
		t.Errorf("stderr lacks 'dirty': %s", stderr)
	}
}

func TestHeadMovedBetweenStageAndApply(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "x\n"})
	mustRunCLI(t, r, 0, "stage")
	r.CommitAs("Bob", "bob@example.com", map[string]string{"a.txt": "x\ny\n"})
	r.WriteFiles(map[string]string{"a.txt": "X\nY\n"})
	code, _, stderr := r.RunCLI("apply")
	if code != 2 {
		t.Fatalf("expected exit 2 for moved HEAD, got %d (stderr=%s)", code, stderr)
	}
	if !strings.Contains(stderr, "HEAD moved") {
		t.Errorf("stderr lacks 'HEAD moved': %s", stderr)
	}
}

func TestStatusAndAbort(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{"a.txt": "x\n"})
	_, stdout, _ := r.RunCLI("status")
	if !strings.Contains(stdout, "no snapshot") {
		t.Errorf("expected 'no snapshot', got %q", stdout)
	}
	mustRunCLI(t, r, 0, "stage")
	_, stdout, _ = r.RunCLI("status")
	if !strings.Contains(stdout, "snapshot at") {
		t.Errorf("expected 'snapshot at', got %q", stdout)
	}
	mustRunCLI(t, r, 0, "abort")
	_, stdout, _ = r.RunCLI("status")
	if !strings.Contains(stdout, "no snapshot") {
		t.Errorf("after abort expected 'no snapshot', got %q", stdout)
	}
}

func TestDryRun(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"a.txt": "alice\nKEEP\n",
	})
	r.CommitAs("Bob", "bob@example.com", map[string]string{
		"a.txt": "alice\nKEEP\nbob\n",
	})
	mustRunCLI(t, r, 0, "stage")
	preHead := r.HeadSHA()
	r.WriteFiles(map[string]string{"a.txt": "ALICE\nKEEP\nBOB\n"})
	code, stdout, _ := r.RunCLI("apply", "--dry-run")
	if code != 0 {
		t.Fatalf("apply --dry-run exit = %d", code)
	}
	if !strings.Contains(stdout, "alice@example.com") || !strings.Contains(stdout, "bob@example.com") {
		t.Errorf("dry-run plan should list both authors: %s", stdout)
	}
	if r.HeadSHA() != preHead {
		t.Fatalf("dry-run must not advance HEAD")
	}
}

func TestSmokeMultiFile(t *testing.T) {
	r := repotest.New(t)
	r.CommitAs("Alice", "alice@example.com", map[string]string{
		"f_0.txt": "alice writes f0\nKEEP\n",
		"f_1.txt": "alice writes f1\nKEEP\n",
		"f_2.txt": "alice writes f2\nKEEP\n",
	})
	r.CommitAs("Bob", "bob@example.com", map[string]string{
		"f_0.txt": "alice writes f0\nKEEP\nbob adds f0\n",
		"f_3.txt": "bob writes f3\n",
	})
	r.CommitAs("Carol", "carol@example.com", map[string]string{
		"f_1.txt": "alice writes f1\nKEEP\ncarol adds f1\n",
		"f_4.txt": "carol writes f4\n",
	})
	mustRunCLI(t, r, 0, "stage")

	reformat := map[string]string{}
	for _, f := range []string{"f_0.txt", "f_1.txt", "f_2.txt", "f_3.txt", "f_4.txt"} {
		reformat[f] = strings.ToUpper(r.Read(f))
	}
	r.WriteFiles(reformat)

	mustRunCLI(t, r, 0, "apply")

	wantFinal := map[string]string{
		"f_0.txt": "ALICE WRITES F0\nKEEP\nBOB ADDS F0\n",
		"f_1.txt": "ALICE WRITES F1\nKEEP\nCAROL ADDS F1\n",
		"f_2.txt": "ALICE WRITES F2\nKEEP\n",
		"f_3.txt": "BOB WRITES F3\n",
		"f_4.txt": "CAROL WRITES F4\n",
	}
	for path, want := range wantFinal {
		if got := r.Read(path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	emails := r.CommitEmails(5)
	joined := strings.Join(emails, ",")
	for _, want := range []string{"alice@example.com", "bob@example.com", "carol@example.com"} {
		if !strings.Contains(joined, want) {
			t.Errorf("commit log missing %s; got %v", want, emails)
		}
	}
}

func mustRunCLI(t *testing.T, r *repotest.Repo, want int, args ...string) {
	t.Helper()
	code, stdout, stderr := r.RunCLI(args...)
	if code != want {
		t.Fatalf("unblamed %v: exit %d (want %d)\nstdout: %s\nstderr: %s", args, code, want, stdout, stderr)
	}
}
