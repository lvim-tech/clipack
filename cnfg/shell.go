package cnfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Shell is one shell clipack knows how to extend: where it keeps the file it
// reads on startup, and how that file spells "put this directory first on the
// path".
//
// The set below is deliberately wider than the three shells clipack used to
// handle. Only one of them is ever written to — the one clipack was started
// from — so a user who moves between shells extends each in turn, from that
// shell, rather than having clipack guess at files for shells they do not run.
type Shell struct {
	Name string
	// rc is the startup file, as path elements below the home directory. Shells
	// that follow the XDG layout are marked by xdg, and then the elements are
	// resolved below $XDG_CONFIG_HOME instead.
	rc  []string
	xdg bool
	// exports renders the lines that prepend binPath and manPath. Every shell
	// here has its own syntax for it, and three of them cannot express it as a
	// plain assignment at all.
	exports func(binPath, manPath string) string
}

// shells lists every shell clipack can extend, in no particular order: lookup
// is by name.
var shells = []Shell{
	// POSIX-family shells. All of them accept the same two exports; they differ
	// only in which file they read.
	{Name: "bash", rc: []string{".bashrc"}, exports: posixExports},
	{Name: "zsh", rc: []string{".zshrc"}, exports: posixExports},
	{Name: "ksh", rc: []string{".kshrc"}, exports: posixExports},
	{Name: "ksh93", rc: []string{".kshrc"}, exports: posixExports},
	{Name: "loksh", rc: []string{".kshrc"}, exports: posixExports},
	{Name: "oksh", rc: []string{".kshrc"}, exports: posixExports},
	{Name: "mksh", rc: []string{".mkshrc"}, exports: posixExports},
	{Name: "yash", rc: []string{".yashrc"}, exports: posixExports},
	// dash, ash and sh have no interactive rc file of their own that they read
	// unconditionally, so the login profile is the only reliable place.
	{Name: "dash", rc: []string{".profile"}, exports: posixExports},
	{Name: "ash", rc: []string{".profile"}, exports: posixExports},
	{Name: "sh", rc: []string{".profile"}, exports: posixExports},

	{Name: "fish", rc: []string{"fish", "config.fish"}, xdg: true, exports: fishExports},

	{Name: "csh", rc: []string{".cshrc"}, exports: cshExports},
	{Name: "tcsh", rc: []string{".tcshrc"}, exports: cshExports},

	{Name: "nu", rc: []string{"nushell", "env.nu"}, xdg: true, exports: nuExports},
	{Name: "elvish", rc: []string{"elvish", "rc.elv"}, xdg: true, exports: elvishExports},
	{Name: "xonsh", rc: []string{".xonshrc"}, exports: xonshExports},
	{Name: "pwsh", rc: []string{"powershell", "Microsoft.PowerShell_profile.ps1"}, xdg: true, exports: pwshExports},
	{Name: "powershell", rc: []string{"powershell", "Microsoft.PowerShell_profile.ps1"}, xdg: true, exports: pwshExports},
}

// posixExports renders the bash/zsh/ksh form. An unset MANPATH leaves a
// trailing colon, which man reads as "and then the system default" — which is
// what is wanted.
func posixExports(binPath, manPath string) string {
	return fmt.Sprintf("\nexport PATH=\"%s:$PATH\"\nexport MANPATH=\"%s:$MANPATH\"\n", binPath, manPath)
}

func fishExports(binPath, manPath string) string {
	return fmt.Sprintf("\nset -x PATH %s $PATH\nset -x MANPATH %s $MANPATH\n", binPath, manPath)
}

// cshExports guards the MANPATH line. csh treats a reference to an unset
// variable as a hard error rather than an empty string, so the unguarded form
// aborts the whole rc file on any machine where MANPATH is not already set —
// taking the PATH line above it with it.
func cshExports(binPath, manPath string) string {
	return fmt.Sprintf(`
setenv PATH "%s:$PATH"
if ( $?MANPATH ) then
	setenv MANPATH "%s:$MANPATH"
else
	setenv MANPATH "%s"
endif
`, binPath, manPath, manPath)
}

// nuExports uses nushell's list-valued PATH. MANPATH is a plain string there,
// and the ? suffix supplies an empty one when it is unset.
func nuExports(binPath, manPath string) string {
	return fmt.Sprintf("\n$env.PATH = ($env.PATH | prepend '%s')\n$env.MANPATH = $'%s:($env.MANPATH? | default \"\")'\n",
		binPath, manPath)
}

// elvishExports assigns through $paths, elvish's list view of PATH. $E: reads
// an unset environment variable as the empty string, so MANPATH needs no guard.
func elvishExports(binPath, manPath string) string {
	return fmt.Sprintf("\nset paths = ['%s' $@paths]\nset E:MANPATH = '%s:'$E:MANPATH\n", binPath, manPath)
}

// xonshExports goes through the environment object for MANPATH: $MANPATH raises
// KeyError when it is unset, and ${...} is xonsh's handle on the whole env.
func xonshExports(binPath, manPath string) string {
	return fmt.Sprintf("\n$PATH.insert(0, '%s')\n$MANPATH = '%s:' + ${...}.get('MANPATH', '')\n", binPath, manPath)
}

func pwshExports(binPath, manPath string) string {
	return fmt.Sprintf("\n$env:PATH = \"%s\" + [IO.Path]::PathSeparator + $env:PATH\n"+
		"$env:MANPATH = \"%s\" + [IO.Path]::PathSeparator + $env:MANPATH\n", binPath, manPath)
}

// SupportedShells names every shell clipack can extend, for an error message
// that says what the alternatives are.
func SupportedShells() []string {
	names := make([]string, 0, len(shells))
	for _, sh := range shells {
		names = append(names, sh.Name)
	}
	return names
}

// ShellOverrideEnv names the shell explicitly, for the case where clipack
// guesses wrong — a shell reached through a wrapper, or one whose name on disk
// is not the name in the table.
const ShellOverrideEnv = "CLIPACK_SHELL"

// CurrentShell returns the shell clipack is actually running under.
//
// The process tree is asked before $SHELL, which is the opposite of the obvious
// order and the whole reason this works: $SHELL names the *login* shell and
// does not change when you type "bash" inside zsh. Keying on it meant clipack,
// started from bash, offered to extend ~/.zshrc — the one file that was already
// correct. The parent process is the shell that actually ran it.
//
// $SHELL still covers what the tree cannot: a system without /proc, and clipack
// started straight from a terminal emulator with no shell in between.
func CurrentShell() (Shell, error) {
	if name := strings.TrimSpace(os.Getenv(ShellOverrideEnv)); name != "" {
		if sh, ok := lookupShell(filepath.Base(name)); ok {
			return sh, nil
		}
		return Shell{}, fmt.Errorf("unsupported shell: %s (from %s)", name, ShellOverrideEnv)
	}

	if sh, ok := lookupShell(parentShell()); ok {
		return sh, nil
	}

	value := strings.TrimSpace(os.Getenv("SHELL"))
	if value == "" {
		return Shell{}, fmt.Errorf("cannot tell which shell you are using: $SHELL is not set")
	}
	if sh, ok := lookupShell(filepath.Base(value)); ok {
		return sh, nil
	}
	return Shell{}, fmt.Errorf("unsupported shell: %s", value)
}

// lookupShell resolves a program name to a known shell.
func lookupShell(name string) (Shell, bool) {
	// A login shell is conventionally argv[0]-prefixed with a dash, and that
	// prefix reaches both /proc and, on some systems, $SHELL.
	name = strings.TrimPrefix(strings.TrimSpace(name), "-")
	if name == "" {
		return Shell{}, false
	}
	for _, sh := range shells {
		if sh.Name == name {
			return sh, true
		}
	}
	return Shell{}, false
}

// parentShell names the process that launched clipack, when that process is a
// shell clipack knows, and "" otherwise. Linux only; every other system gets ""
// and falls back to $SHELL.
//
// Deliberately the immediate parent and no further. Walking up the tree looks
// more thorough and is worse: the first ancestor that happens to be a shell is
// not necessarily the one you are typing into. Under `go test` the chain is
// test binary → go → the developer's shell, and a walking version answered
// "zsh" for a process no shell had launched. If the launcher is not a shell,
// clipack does not know, and saying so is what $SHELL is the fallback for.
func parentShell() string {
	name, err := procName(os.Getppid())
	if err != nil {
		return ""
	}
	if _, ok := lookupShell(name); ok {
		return name
	}
	return ""
}

// procName reads a process's program name from /proc.
func procName(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// configHome is where shells that follow the XDG layout keep their files.
func configHome(home string) string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return dir
	}
	return filepath.Join(home, ".config")
}

// RCFile is the startup file this shell reads, as an absolute path.
//
// The home directory comes from os.UserHomeDir rather than os/user, so the rc
// file and the configuration in ConfigDir always agree on where "home" is —
// they used to disagree whenever $HOME differed from the passwd entry.
func (s Shell) RCFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}
	if s.xdg {
		return filepath.Join(append([]string{configHome(home)}, s.rc...)...), nil
	}
	return filepath.Join(append([]string{home}, s.rc...)...), nil
}

// ExportLines returns the lines that put binPath and manPath on this shell's
// path.
func (s Shell) ExportLines(binPath, manPath string) string {
	return s.exports(binPath, manPath)
}

// ShellExportLines returns the lines that put bin and man on the current
// shell's path.
func ShellExportLines(binPath, manPath string) (string, error) {
	sh, err := CurrentShell()
	if err != nil {
		return "", err
	}
	return sh.ExportLines(binPath, manPath), nil
}

// GetShellConfigFilePath returns the startup file of the current shell.
func GetShellConfigFilePath() (string, error) {
	sh, err := CurrentShell()
	if err != nil {
		return "", err
	}
	return sh.RCFile()
}

// ShellStatus describes how the current shell stands with respect to clipack's
// bin directory: whether the running shell can already find it, and whether the
// file that would arrange that mentions it.
//
// The two are tracked separately because they disagree in the ordinary case:
// right after the rc file is written, the session that wrote it still has the
// old PATH. Reporting that as "not set up" would invite the user to add it
// again, which does nothing.
type ShellStatus struct {
	Shell  Shell
	RCFile string
	OnPath bool
	InRC   bool
}

// NeedsPaths reports whether there is anything to do: the shell cannot find
// binPath and nothing in its startup file will change that.
func (s ShellStatus) NeedsPaths() bool { return !s.OnPath && !s.InRC }

// NeedsRestart reports that the startup file is already correct and only this
// session is behind.
func (s ShellStatus) NeedsRestart() bool { return !s.OnPath && s.InRC }

// CurrentShellStatus reports where the current shell stands with respect to
// binPath. It is the check behind the warning in the interface.
func CurrentShellStatus(binPath string) (ShellStatus, error) {
	sh, err := CurrentShell()
	if err != nil {
		return ShellStatus{}, err
	}

	rc, err := sh.RCFile()
	if err != nil {
		return ShellStatus{}, err
	}

	status := ShellStatus{Shell: sh, RCFile: rc, OnPath: onPath(binPath)}

	// A missing rc file is not an error here: it simply does not reference the
	// bin directory yet, which is exactly what the caller is asking about.
	if contents, err := os.ReadFile(rc); err == nil {
		status.InRC = strings.Contains(string(contents), binPath)
	}
	return status, nil
}

// onPath reports whether dir is one of the directories the current process
// would search for a program.
func onPath(dir string) bool {
	if dir == "" {
		return false
	}
	want := filepath.Clean(dir)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == "" {
			continue
		}
		if filepath.Clean(entry) == want {
			return true
		}
	}
	return false
}

// AddPathsToShell appends the bin and man exports to the current shell's
// startup file and reports which file it wrote. It is a no-op when the file
// already references binPath, so running it repeatedly does not stack duplicate
// exports.
//
// Unlike AddPathsToShellConfig it prints nothing, which is what lets the
// interface call it without writing over its own frame.
func AddPathsToShell(binPath, manPath string) (ShellStatus, error) {
	status, err := CurrentShellStatus(binPath)
	if err != nil {
		return ShellStatus{}, err
	}
	if status.InRC {
		return status, nil
	}

	if err := os.MkdirAll(filepath.Dir(status.RCFile), 0o755); err != nil {
		return status, fmt.Errorf("could not create shell config directory: %w", err)
	}

	file, err := os.OpenFile(status.RCFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return status, fmt.Errorf("could not open shell config file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(status.Shell.ExportLines(binPath, manPath)); err != nil {
		return status, fmt.Errorf("could not write to shell config file: %w", err)
	}

	status.InRC = true
	return status, nil
}

// AddPathsToShellConfig appends the bin and man paths to the current shell's
// startup file and reports what it did on stdout. It is the form the CLI uses.
func AddPathsToShellConfig(binPath, manPath string) error {
	before, err := CurrentShellStatus(binPath)
	if err != nil {
		return fmt.Errorf("could not determine shell config file path: %w", err)
	}
	if before.InRC {
		fmt.Printf("%s already references %s, nothing to do\n", before.RCFile, binPath)
		return nil
	}

	status, err := AddPathsToShell(binPath, manPath)
	if err != nil {
		return err
	}

	fmt.Printf("Paths added to %s\n", status.RCFile)
	return nil
}
