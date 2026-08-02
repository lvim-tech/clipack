package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
)

// shellHome points both $HOME and $XDG_CONFIG_HOME at a temporary directory, so
// a test that writes an rc file cannot reach the one the developer uses.
func shellHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// shellStatusFor builds the status the check command would produce for a shell,
// without depending on which shells the machine running the tests has.
func shellStatusFor(t *testing.T, shell, binPath string) cnfg.ShellStatus {
	t.Helper()

	t.Setenv(cnfg.ShellOverrideEnv, shell)
	t.Setenv("PATH", "/usr/bin:/bin")

	status, err := cnfg.CurrentShellStatus(binPath)
	if err != nil {
		t.Fatalf("CurrentShellStatus() error = %v", err)
	}
	return status
}

func TestShellNoticeWarnsWhenTheShellCannotFindBin(t *testing.T) {
	shellHome(t)
	m := browseModel(t)

	status := shellStatusFor(t, "/usr/bin/zsh", m.config.Paths.Bin)
	m = applyMsg(t, m, shellPathMsg{status: status})

	notice, ok := m.shellNotice()
	if !ok {
		t.Fatal("shellNotice() reported nothing, want a warning for a shell without the bin directory")
	}
	for _, want := range []string{"zsh", m.config.Paths.Bin, status.RCFile, "press p"} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice = %q, want it to mention %q", notice, want)
		}
	}

	// The warning is a header line, so the header has to have grown by one.
	if !strings.Contains(m.View(), "press p") {
		t.Error("View() does not carry the warning, want it visible on the browse screen")
	}
}

// TestShellNoticeIsSilentWhenBinIsOnPath is the case that must stay quiet: a
// shell that can already find the directory has nothing to be told.
func TestShellNoticeIsSilentWhenBinIsOnPath(t *testing.T) {
	shellHome(t)
	m := browseModel(t)

	t.Setenv(cnfg.ShellOverrideEnv, "/usr/bin/zsh")
	t.Setenv("PATH", strings.Join([]string{"/usr/bin", m.config.Paths.Bin}, string(os.PathListSeparator)))

	status, err := cnfg.CurrentShellStatus(m.config.Paths.Bin)
	if err != nil {
		t.Fatalf("CurrentShellStatus() error = %v", err)
	}
	m = applyMsg(t, m, shellPathMsg{status: status})

	if notice, ok := m.shellNotice(); ok {
		t.Errorf("shellNotice() = %q, want nothing when the directory is already on PATH", notice)
	}
	if m.needsShellPath() {
		t.Error("needsShellPath() = true, want false when the directory is already on PATH")
	}
}

// TestShellNoticeIsSilentForAnUnknownShell covers the shell clipack cannot
// write to. Warning there would offer a key that could not do anything.
func TestShellNoticeIsSilentForAnUnknownShell(t *testing.T) {
	shellHome(t)
	m := browseModel(t)

	t.Setenv(cnfg.ShellOverrideEnv, "/usr/bin/shellwedonotknow")
	_, err := cnfg.CurrentShellStatus(m.config.Paths.Bin)
	if err == nil {
		t.Fatal("CurrentShellStatus() error = nil, want an error for an unknown shell")
	}
	m = applyMsg(t, m, shellPathMsg{err: err})

	if notice, ok := m.shellNotice(); ok {
		t.Errorf("shellNotice() = %q, want nothing for a shell clipack cannot write to", notice)
	}
	if m.contextualKeys().Path.Enabled() {
		t.Error("the p key is enabled for an unknown shell, want it disabled")
	}
}

// TestShellNoticeAsksForARestartOnceWritten separates "there is something to do"
// from "you did it". Without it the red warning survives the keypress, because
// this session's PATH cannot change, and the key looks broken.
func TestShellNoticeAsksForARestartOnceWritten(t *testing.T) {
	home := shellHome(t)
	m := browseModel(t)

	t.Setenv(cnfg.ShellOverrideEnv, "/bin/bash")
	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := cnfg.AddPathsToShell(m.config.Paths.Bin, m.config.Paths.Man, ""); err != nil {
		t.Fatalf("AddPathsToShell() error = %v", err)
	}

	status, err := cnfg.CurrentShellStatus(m.config.Paths.Bin)
	if err != nil {
		t.Fatalf("CurrentShellStatus() error = %v", err)
	}
	m = applyMsg(t, m, shellPathMsg{status: status})

	notice, ok := m.shellNotice()
	if !ok {
		t.Fatal("shellNotice() reported nothing, want the restart hint")
	}
	if strings.Contains(notice, "press p") {
		t.Errorf("notice = %q, want the restart hint rather than the offer", notice)
	}
	if !strings.Contains(notice, "restart") {
		t.Errorf("notice = %q, want it to ask for a restart", notice)
	}
	if m.needsShellPath() {
		t.Error("needsShellPath() = true after the rc file was written, want false")
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); err != nil {
		t.Errorf("stat .bashrc = %v, want the file written into the test home", err)
	}
}

// TestPathKeyWritesTheCurrentShellsFile drives the whole thing through the key,
// which is the only path a user takes.
func TestPathKeyWritesTheCurrentShellsFile(t *testing.T) {
	home := shellHome(t)
	m := browseModel(t)

	status := shellStatusFor(t, "/bin/bash", m.config.Paths.Bin)
	m = applyMsg(t, m, shellPathMsg{status: status})
	if !m.needsShellPath() {
		t.Fatal("needsShellPath() = false, want true before the key is pressed")
	}

	m, cmd := applyMsgCmd(t, m, keyMsg("p"))
	if cmd == nil {
		t.Fatal("pressing p produced no command, want the rc file written")
	}

	// Running the command is what does the work; its message closes the loop.
	m = applyMsg(t, m, cmd())

	contents, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf(".bashrc was not written: %v", err)
	}
	if !strings.Contains(string(contents), m.config.Paths.Bin) {
		t.Errorf(".bashrc = %q, want it to reference the bin directory", contents)
	}
	if m.needsShellPath() {
		t.Error("needsShellPath() = true after pressing p, want the warning gone")
	}
	if !strings.Contains(m.status, ".bashrc") {
		t.Errorf("status = %q, want it to name the file that was written", m.status)
	}
}

// TestPathKeyIsInertWithNothingToDo guards the rc file: p is a free key
// everywhere else, and an unguarded handler would append the exports again.
func TestPathKeyIsInertWithNothingToDo(t *testing.T) {
	shellHome(t)
	m := browseModel(t)

	t.Setenv(cnfg.ShellOverrideEnv, "/bin/bash")
	t.Setenv("PATH", strings.Join([]string{"/usr/bin", m.config.Paths.Bin}, string(os.PathListSeparator)))
	status, err := cnfg.CurrentShellStatus(m.config.Paths.Bin)
	if err != nil {
		t.Fatalf("CurrentShellStatus() error = %v", err)
	}
	m = applyMsg(t, m, shellPathMsg{status: status})

	if _, cmd := applyMsgCmd(t, m, keyMsg("p")); cmd != nil {
		t.Error("pressing p produced a command with nothing to do, want none")
	}
}

// TestPathKeyAppearsInTheHelpOnlyWhenItApplies ties the key to the warning: the
// help line lists the keys that are live, and one without the other is a key
// nobody knows about or a key that does nothing.
func TestPathKeyAppearsInTheHelpOnlyWhenItApplies(t *testing.T) {
	shellHome(t)
	m := browseModel(t)

	if m.contextualKeys().Path.Enabled() {
		t.Error("the p key is enabled before the shell has been checked, want it disabled")
	}

	status := shellStatusFor(t, "/usr/bin/fish", m.config.Paths.Bin)
	m = applyMsg(t, m, shellPathMsg{status: status})

	keys := m.contextualKeys()
	if !keys.Path.Enabled() {
		t.Fatal("the p key is disabled while the warning is on screen, want it enabled")
	}
	// The label names the shell, because which shell this is about is the whole
	// point: the answer differs per shell.
	if desc := keys.Path.Help().Desc; !strings.Contains(desc, "fish") {
		t.Errorf("help = %q, want it to name the shell it will write to", desc)
	}
}
