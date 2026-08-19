package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/utils"
)

// Exposing is how a binary clipack built becomes a command the shell can find.
//
// clipack's bin directory holds everything it has ever built and is meant to
// stay off PATH: putting eighty programs into the environment at once shadows
// the distribution's tools, and does it silently. So visibility is granted one
// name at a time, by a symlink in a directory that is already on PATH — which
// is exactly what was being done by hand, and what this automates without
// changing the rule it follows.
//
// The rule the code has to keep is that clipack owns its own links and nothing
// else. Every operation here looks at what the name currently holds before it
// writes or deletes: a link clipack would have made is adopted or repaired, and
// anything else is left where it is and reported.

// ExposeState is what the expose directory currently holds for one name.
type ExposeState int

const (
	// ExposeAbsent: nothing holds the name.
	ExposeAbsent ExposeState = iota
	// ExposeLinked: a symlink pointing at exactly the binary it should.
	// A link made by hand is indistinguishable from one clipack made, which is
	// the point — it is adopted rather than recreated.
	ExposeLinked
	// ExposeStale: a symlink into clipack's bin directory, but not at this
	// package's binary. An installation that moved, or a name that changed.
	ExposeStale
	// ExposeForeign: a file, a directory, or a link pointing somewhere else.
	// Not clipack's to touch.
	ExposeForeign
)

// String renders the state for a message.
func (s ExposeState) String() string {
	switch s {
	case ExposeLinked:
		return "linked"
	case ExposeStale:
		return "stale"
	case ExposeForeign:
		return "foreign"
	default:
		return "absent"
	}
}

// ExposeStatus describes one exposed name: where its link is, where it should
// point, where it points now, and what would stop it from working.
type ExposeStatus struct {
	// Name is the binary as it appears in the bin directory.
	Name string
	// Link is the path in the expose directory.
	Link string
	// Target is where the link should point: <bin>/<name>.
	Target string
	// Points is where the link points now, empty when there is no link.
	Points string
	// State is how Points compares to Target.
	State ExposeState
	// Declared is set when install.expose names this binary, as opposed to it
	// having been exposed by hand.
	Declared bool
	// Known is false when the package does not install a binary by this name,
	// which makes the entry a typo rather than a link.
	Known bool
	// Shadow is a directory earlier on PATH holding a program of the same
	// name. The link is then correct and still never reached.
	Shadow string
	// DirOnPath reports whether the expose directory is on PATH at all.
	DirOnPath bool
}

// OK reports whether the name is linked, reachable and unshadowed.
func (s ExposeStatus) OK() bool {
	return s.Known && s.State == ExposeLinked && s.Shadow == "" && s.DirOnPath
}

// Problem states what is wrong with this entry in one line, or "" when nothing
// is. Written once here so the CLI and the interface say the same thing.
func (s ExposeStatus) Problem() string {
	switch {
	case !s.Known:
		return "not a binary this package installs"
	case s.State == ExposeForeign:
		if s.Points != "" {
			return fmt.Sprintf("%s already points at %s", s.Link, s.Points)
		}
		return fmt.Sprintf("%s exists and is not a clipack link", s.Link)
	case s.State == ExposeAbsent:
		return "not linked"
	case s.State == ExposeStale:
		return fmt.Sprintf("points at %s", s.Points)
	case s.Shadow != "":
		return fmt.Sprintf("shadowed by %s, earlier on PATH", filepath.Join(s.Shadow, s.Name))
	case !s.DirOnPath:
		return fmt.Sprintf("%s is not on PATH", filepath.Dir(s.Link))
	default:
		return ""
	}
}

// BinaryNames lists the executables a package puts in the bin directory: the
// base names of its binaries, plus its post-install scripts, which land there
// the same way and are just as legitimate a thing to expose.
func (p *Package) BinaryNames() []string {
	var names []string
	for _, bin := range p.Install.Binaries {
		names = appendName(names, filepath.Base(bin))
	}
	for _, script := range p.PostInstall.Scripts {
		names = appendName(names, filepath.Base(script.Filename))
	}
	return names
}

// ExposeNames is the set of binaries this installation exposes: what the
// registry entry declares, plus what was exposed by hand, minus what was
// unexposed by hand.
//
// Order follows the registry first and the ad-hoc additions after, so the list
// reads the way it was built up rather than alphabetically.
func (p *Package) ExposeNames() []string {
	var names []string
	for _, name := range p.Install.Expose {
		if !containsName(p.Unexposed, name) {
			names = appendName(names, name)
		}
	}
	for _, name := range p.Exposed {
		if !containsName(p.Unexposed, name) {
			names = appendName(names, name)
		}
	}
	return names
}

// UnknownExpose returns the names in the given list that the package does not
// install. Reported rather than skipped: a typo in install.expose is otherwise
// invisible until someone wonders why the command is missing.
func UnknownExpose(p *Package, names []string) []string {
	known := p.BinaryNames()
	var unknown []string
	for _, name := range names {
		if !containsName(known, name) {
			unknown = appendName(unknown, name)
		}
	}
	return unknown
}

// ExposeStatuses reports what every name this package exposes looks like on
// disk. It is what the interface draws and what the CLI prints.
func ExposeStatuses(config *cnfg.Config, p *Package) []ExposeStatus {
	names := p.ExposeNames()
	if len(names) == 0 {
		return nil
	}

	exposeDir := config.Paths.Expose
	dirOnPath := cnfg.DirOnPath(exposeDir) >= 0
	known := p.BinaryNames()

	statuses := make([]ExposeStatus, 0, len(names))
	for _, name := range names {
		st := ExposeStatus{
			Name:      name,
			Target:    filepath.Join(config.Paths.Bin, name),
			Declared:  containsName(p.Install.Expose, name),
			Known:     containsName(known, name),
			DirOnPath: dirOnPath,
		}
		if exposeDir != "" {
			st.Link = filepath.Join(exposeDir, name)
			st.State, st.Points = exposeState(st.Link, st.Target, config.Paths.Bin)
			st.Shadow = shadowedBy(name, exposeDir)
		}
		statuses = append(statuses, st)
	}
	return statuses
}

// exposeState classifies what link currently is, and returns where it points.
func exposeState(link, target, binDir string) (ExposeState, string) {
	info, err := os.Lstat(link)
	if err != nil {
		return ExposeAbsent, ""
	}
	if info.Mode()&os.ModeSymlink == 0 {
		// A real file or a directory. Whatever put it there, it was not clipack.
		return ExposeForeign, ""
	}

	points, err := os.Readlink(link)
	if err != nil {
		return ExposeForeign, ""
	}
	if !filepath.IsAbs(points) {
		points = filepath.Join(filepath.Dir(link), points)
	}
	points = filepath.Clean(points)

	switch {
	case points == filepath.Clean(target):
		return ExposeLinked, points
	case filepath.Dir(points) == filepath.Clean(binDir):
		// Into clipack's bin directory but at the wrong name — an installation
		// that moved, or a package that renamed its binary.
		return ExposeStale, points
	default:
		return ExposeForeign, points
	}
}

// shadowedBy returns the directory that would win the lookup for name, when it
// comes before exposeDir on PATH. Empty when nothing shadows the link.
//
// A correct link in a directory that is on PATH is still not the program that
// runs when something earlier on PATH answers to the same name — which is
// precisely the failure exposing is meant to prevent, so it is worth saying.
func shadowedBy(name, exposeDir string) string {
	if name == "" || exposeDir == "" {
		return ""
	}
	// The answer is only known once exposeDir itself has been reached: a
	// directory holding the name shadows the link when it comes first, and
	// means nothing at all when the link's directory is not on PATH — which the
	// caller reports as its own, different problem.
	want := filepath.Clean(exposeDir)
	shadow := ""
	for _, dir := range cnfg.PathDirs() {
		if filepath.Clean(dir) == want {
			return shadow
		}
		if shadow == "" && isExecutable(filepath.Join(dir, name)) {
			shadow = dir
		}
	}
	return ""
}

// isExecutable reports whether path is something the shell would run.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

// linkExpose makes link point at target, and returns the state it found.
//
// Absent is created, an identical link is adopted without a word, and a link
// into clipack's own bin directory is repaired. Anything else is left alone and
// returned as ExposeForeign, for the caller to report — clipack deleting a file
// it did not create, in a directory it does not own, is the one outcome that
// would be worse than the command staying invisible.
func linkExpose(link, target, binDir string) (ExposeState, string, error) {
	state, points := exposeState(link, target, binDir)
	switch state {
	case ExposeLinked, ExposeForeign:
		return state, points, nil
	}
	return state, points, replaceSymlink(target, link)
}

// unlinkExpose removes link when, and only when, it points at target.
//
// Everything else stays: a file somebody else owns, and a link into clipack's
// bin directory that names a different binary, are both somebody else's to
// remove. The state found is returned so the caller can say which it was.
func unlinkExpose(link, target, binDir string) (ExposeState, error) {
	state, _ := exposeState(link, target, binDir)
	if state != ExposeLinked {
		return state, nil
	}
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return state, err
	}
	return state, nil
}

// replaceSymlink points link at target, atomically.
//
// The link is created under a neighbouring name and renamed over the
// destination, for the same reason CopyFile does it: remove-then-create leaves
// a window in which the command does not exist, and an interrupted operation
// leaves it missing rather than stale. rename(2) replaces the directory entry
// in one step, so the name is either the old link or the new one.
func replaceSymlink(target, link string) error {
	dir := filepath.Dir(link)
	if err := utils.EnsureDirectoryExists(dir); err != nil {
		return err
	}

	// CreateTemp reserves a name nothing else will take; the file itself is
	// removed straight away, because a symlink cannot be created over it.
	temp, err := os.CreateTemp(dir, "."+filepath.Base(link)+".clipack-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	temp.Close()
	defer os.Remove(tempName) // a no-op once the rename has taken the name away

	if err := os.Remove(tempName); err != nil {
		return err
	}
	if err := os.Symlink(target, tempName); err != nil {
		return err
	}
	return os.Rename(tempName, link)
}

// applyExpose creates the links this package's install has earned.
//
// Nothing here fails an install. A program that is built and in bin/ is
// installed; a link that could not be made is a name the shell will not find,
// which is worth a warning and not worth undoing a build over.
func (in *Installer) applyExpose(p *Package, paths Paths) {
	names := p.ExposeNames()
	if len(names) == 0 {
		return
	}
	if paths.Expose == "" {
		in.warnf("%s asks to expose %s, but no expose directory is configured (paths.expose)",
			p.Name, strings.Join(names, ", "))
		return
	}

	if unknown := UnknownExpose(p, names); len(unknown) > 0 {
		in.warnf("%s exposes %s, which it does not install (it installs %s)",
			p.Name, strings.Join(unknown, ", "), strings.Join(p.BinaryNames(), ", "))
	}

	known := p.BinaryNames()
	dirOnPath := cnfg.DirOnPath(paths.Expose) >= 0
	for _, name := range names {
		if !containsName(known, name) {
			continue
		}
		link := filepath.Join(paths.Expose, name)
		target := filepath.Join(paths.Bin, name)

		state, points, err := linkExpose(link, target, paths.Bin)
		if err != nil {
			in.warnf("could not expose %s: %v", name, err)
			continue
		}
		switch state {
		case ExposeAbsent:
			in.infof("Exposed %s → %s", link, target)
		case ExposeStale:
			in.infof("Repointed %s → %s", link, target)
		case ExposeForeign:
			if points != "" {
				in.warnf("not exposing %s: %s already points at %s", name, link, points)
			} else {
				in.warnf("not exposing %s: %s exists and is not a clipack link", name, link)
			}
			continue
		}
		// ExposeLinked falls through silently: the link is already what it
		// should be, whoever made it.

		// A link nothing reaches is worth as much as no link, so the two ways
		// that happens are said at the moment the link is made.
		if shadow := shadowedBy(name, paths.Expose); shadow != "" {
			in.warnf("%s is exposed but shadowed by %s, earlier on PATH",
				name, filepath.Join(shadow, name))
		} else if !dirOnPath {
			in.warnf("%s is exposed in %s, which is not on PATH", name, paths.Expose)
		}
	}
}

// removeExposed drops the links this package's install created. Only the ones
// that point at its own binaries: a file of the same name that belongs to
// something else survives the uninstall, which is the whole reason the target
// is read before anything is deleted.
func (in *Installer) removeExposed(p *Package, paths Paths) {
	if paths.Expose == "" {
		return
	}
	for _, name := range p.ExposeNames() {
		link := filepath.Join(paths.Expose, name)
		target := filepath.Join(paths.Bin, name)

		state, err := unlinkExpose(link, target, paths.Bin)
		if err != nil {
			in.warnf("could not remove exposed %s: %v", link, err)
			continue
		}
		switch state {
		case ExposeLinked:
			in.infof("Removed exposed %s", link)
		case ExposeForeign, ExposeStale:
			in.infof("Left %s alone: it is not a link to %s", link, target)
		}
	}
}

// Expose links binaries of an installed package by hand, without touching the
// registry entry.
//
// The choice is written into the manifest, which is what a later rebuild reads:
// an ad-hoc link survives an update the same way the registry's own do. Naming
// no binaries exposes everything the package installs.
func (in *Installer) Expose(p *Package, names []string) error {
	if in.Config.Paths.Expose == "" {
		return fmt.Errorf("no expose directory is configured; set paths.expose in config.yaml")
	}

	produced := p.BinaryNames()
	if len(produced) == 0 {
		return fmt.Errorf("%s installs no binaries to expose", p.Name)
	}
	if len(names) == 0 {
		names = produced
	}
	if unknown := UnknownExpose(p, names); len(unknown) > 0 {
		return fmt.Errorf("%s does not install %s; it installs %s",
			p.Name, strings.Join(unknown, ", "), strings.Join(produced, ", "))
	}

	for _, name := range names {
		p.Exposed = appendName(p.Exposed, name)
		p.Unexposed = removeNameFrom(p.Unexposed, name)
	}

	paths := in.pathsFor(p.Name)
	if err := in.writeManifest(p, paths); err != nil {
		return err
	}
	in.applyExpose(p, paths)
	return nil
}

// Unexpose removes links made for a package, and remembers that it did.
//
// Removing the ad-hoc entry alone would not be enough for a binary the registry
// entry declares: the next rebuild would put the link straight back. So a
// declared name is recorded in Unexposed, which ExposeNames subtracts.
func (in *Installer) Unexpose(p *Package, names []string) error {
	if len(names) == 0 {
		names = p.ExposeNames()
	}
	if len(names) == 0 {
		return fmt.Errorf("%s exposes nothing", p.Name)
	}

	paths := in.pathsFor(p.Name)
	for _, name := range names {
		p.Exposed = removeNameFrom(p.Exposed, name)
		if containsName(p.Install.Expose, name) {
			p.Unexposed = appendName(p.Unexposed, name)
		}

		if paths.Expose == "" {
			continue
		}
		link := filepath.Join(paths.Expose, name)
		target := filepath.Join(paths.Bin, name)
		state, err := unlinkExpose(link, target, paths.Bin)
		if err != nil {
			in.warnf("could not remove exposed %s: %v", link, err)
			continue
		}
		switch state {
		case ExposeLinked:
			in.infof("Removed exposed %s", link)
		case ExposeForeign, ExposeStale:
			in.infof("Left %s alone: it is not a link to %s", link, target)
		}
	}

	return in.writeManifest(p, paths)
}

// containsName reports whether names holds name.
func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// appendName adds name unless it is empty or already there, keeping the order
// the names were added in.
func appendName(names []string, name string) []string {
	if name == "" || containsName(names, name) {
		return names
	}
	return append(names, name)
}

// removeNameFrom drops every occurrence of name, returning nil for an empty
// result so the field is omitted from the manifest rather than written as [].
func removeNameFrom(names []string, name string) []string {
	var out []string
	for _, n := range names {
		if n != name {
			out = append(out, n)
		}
	}
	return out
}
