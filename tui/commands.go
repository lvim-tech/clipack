package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// registryLoadedMsg carries a finished registry load.
// err is non-nil for a hard failure; warn is set when packages were loaded but
// something was still off (e.g. a few files could not be fetched).
type registryLoadedMsg struct {
	packages  []*pkg.Package
	installed map[string]*pkg.Package
	err       error
	warn      error
	refreshed bool
}

// opEventsMsg carries a batch of progress events. Events are batched because a
// compile can emit thousands of lines: delivering them one Bubble Tea round
// trip at a time would re-render the whole log for every single line.
type opEventsMsg struct {
	events   []pkg.Event
	finished bool
	err      error
}

// opFinishedMsg reports that the running operation ended.
type opFinishedMsg struct{ err error }

// setupDoneMsg reports the outcome of the first-run configuration wizard.
type setupDoneMsg struct {
	config *cnfg.Config
	err    error
}

// shellPathMsg carries where the shell clipack was started from stands with
// respect to the managed bin directory. A shell clipack cannot write to is not
// an error worth showing: err simply suppresses the offer.
type shellPathMsg struct {
	status cnfg.ShellStatus
	err    error
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// loadRegistryCmd loads the package list, using the cache when it is fresh.
// refresh forces a network round trip and rewrites the cache.
func loadRegistryCmd(config *cnfg.Config, refresh bool) tea.Cmd {
	return func() tea.Msg {
		var (
			packages []*pkg.Package
			err      error
		)

		if refresh {
			if cerr := pkg.ClearCache(config); cerr != nil {
				return registryLoadedMsg{err: cerr, refreshed: true}
			}
			packages, err = pkg.RefreshRegistry(config)
		} else {
			packages, err = pkg.LoadAllPackagesFromRegistry(config)
		}

		// A partial result is still usable: show it and surface the problem as
		// a warning rather than throwing the whole list away.
		if len(packages) == 0 {
			return registryLoadedMsg{err: err, refreshed: refresh}
		}

		installed, ierr := pkg.InstalledMap(config)
		if ierr != nil {
			return registryLoadedMsg{packages: packages, err: ierr, refreshed: refresh}
		}

		sort.SliceStable(packages, func(i, j int) bool {
			if packages[i].Category != packages[j].Category {
				return packages[i].Category < packages[j].Category
			}
			return packages[i].Name < packages[j].Name
		})

		return registryLoadedMsg{
			packages:  packages,
			installed: installed,
			warn:      err,
			refreshed: refresh,
		}
	}
}

// opStream carries progress events from a running operation to the UI.
type opStream struct {
	events chan pkg.Event
	err    *error
}

// newOpStream starts op in the background and returns a stream of its events.
// The installer reporter writes into a buffered channel; the Bubble Tea update
// loop drains it one message at a time via waitForEventCmd.
func newOpStream(config *cnfg.Config, op func(*pkg.Installer) error) *opStream {
	stream := &opStream{
		events: make(chan pkg.Event, 1024),
		err:    new(error),
	}

	installer := pkg.NewInstaller(config, func(e pkg.Event) {
		stream.events <- e
	})

	go func() {
		// The error is written before the channel is closed, so the receiver
		// observing the close also observes the write.
		*stream.err = op(installer)
		close(stream.events)
	}()

	return stream
}

// maxEventBatch bounds how many events one message may carry, so the UI still
// gets a chance to redraw during a very chatty build.
const maxEventBatch = 128

// waitForEventCmd blocks until at least one event arrives, then drains whatever
// else is already buffered without blocking.
func waitForEventCmd(stream *opStream) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-stream.events
		if !ok {
			return opFinishedMsg{err: *stream.err}
		}

		batch := make([]pkg.Event, 0, maxEventBatch)
		batch = append(batch, first)

		for len(batch) < maxEventBatch {
			select {
			case event, ok := <-stream.events:
				if !ok {
					// Deliver the tail together with the completion, so no
					// output is lost when the operation finishes mid-batch.
					return opEventsMsg{events: batch, finished: true, err: *stream.err}
				}
				batch = append(batch, event)
			default:
				return opEventsMsg{events: batch}
			}
		}

		return opEventsMsg{events: batch}
	}
}

// saveConfigCmd writes a freshly built configuration to disk.
func saveConfigCmd(installDir string, addToShell bool) tea.Cmd {
	return func() tea.Msg {
		config := cnfg.NewDefaultConfig(installDir)
		if err := config.Save(); err != nil {
			return setupDoneMsg{err: err}
		}
		if addToShell {
			// A failure here is not fatal: the config is already usable, and the
			// browse screen offers the same thing again for whichever shell is
			// running. AddPathsToShell rather than AddPathsToShellConfig because
			// the latter prints, and printing over a Bubble Tea frame corrupts it.
			_, _ = cnfg.AddPathsToShell(config.Paths.Bin, config.Paths.Man)
		}
		return setupDoneMsg{config: config}
	}
}

// checkShellPathCmd asks whether the current shell can find the managed bin
// directory. It runs as a command rather than in the constructor so the model
// stays a pure value: nothing is read from the environment or the disk until
// Bubble Tea starts.
func checkShellPathCmd(config *cnfg.Config) tea.Cmd {
	return func() tea.Msg {
		status, err := cnfg.CurrentShellStatus(config.Paths.Bin)
		return shellPathMsg{status: status, err: err}
	}
}

// addShellPathCmd extends the current shell's startup file, and only that one.
// Running clipack from a second shell brings the offer back for that shell.
func addShellPathCmd(config *cnfg.Config) tea.Cmd {
	return func() tea.Msg {
		status, err := cnfg.AddPathsToShell(config.Paths.Bin, config.Paths.Man)
		return shellPathMsg{status: status, err: err}
	}
}
