package pkg

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyFileDoesNotTruncateTheRunningFile is the defect that cost a terminal.
//
// Installing over a file that some process has open used to open the destination with O_TRUNC.
// The bytes a running program is reading come from the INODE, and truncating it pulls them out
// from under a process that has not exited — which is how reinstalling a terminal emulator killed
// the session doing the reinstalling. A rename replaces the directory entry and leaves the inode
// alone, so the old process keeps reading the old file.
//
// The open handle here stands in for that running process: after the copy it must still see every
// byte it saw before.
func TestCopyFileDoesNotTruncateTheRunningFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "program")
	old := []byte("the old program, which something is in the middle of running")
	if err := os.WriteFile(dst, old, 0o755); err != nil {
		t.Fatal(err)
	}

	// Held open the way a running executable holds its own image.
	running, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()

	src := filepath.Join(dir, "new")
	if err := os.WriteFile(src, []byte("the new program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	seen := make([]byte, len(old))
	n, err := running.ReadAt(seen, 0)
	if err != nil {
		t.Fatalf("the open file was cut short after %d of %d bytes: %v", n, len(old), err)
	}
	if string(seen) != string(old) {
		t.Error("the running process's file changed underneath it")
	}
}

// TestCopyFileReplacesTheTarget: the whole point of the copy still has to happen.
func TestCopyFileReplacesTheTarget(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "program")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "new")
	if err := os.WriteFile(src, []byte("new contents"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new contents" {
		t.Errorf("target not replaced: %q", got)
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Errorf("mode not carried over: %v", st.Mode().Perm())
	}
}

// TestCopyFileLeavesNoTemporary: the temporary lives in the destination's directory, so one left
// behind is a stray dotfile next to every binary clipack installs.
func TestCopyFileLeavesNoTemporary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, filepath.Join(dir, "program")); err != nil {
		t.Fatal(err)
	}

	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name() != "new" && e.Name() != "program" {
			t.Errorf("left behind: %s", e.Name())
		}
	}
}
