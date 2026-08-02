package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/pkg"
)

// renderDetail builds the right-hand pane for a package. width is the usable
// inner width of the pane, so long values wrap instead of breaking the layout.
func renderDetail(entry packageItem, method string, width int, s Styles) string {
	if width < 20 {
		width = 20
	}

	p := entry.pkg
	var b strings.Builder

	b.WriteString(s.DetailTitle.Render(p.Name))
	if p.Version != "" {
		b.WriteString(s.Muted.Render("  " + p.Version))
	}
	b.WriteString("\n\n")

	valueWidth := width - detailKeyWidth - 1
	if valueWidth < 10 {
		valueWidth = 10
	}
	wrap := lipgloss.NewStyle().Width(valueWidth)

	row := func(key, value string) {
		if value == "" {
			return
		}
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top,
			s.DetailKey.Render(key),
			wrap.Render(s.DetailValue.Render(value)),
		))
		b.WriteString("\n")
	}

	row("Description", p.Description)
	row("Category", p.Category)
	row("Maintainer", p.Maintainer)
	row("License", p.License)
	row("Homepage", p.Homepage)
	row("Commit", shortCommit(p.Commit))
	if !p.UpdatedAt.IsZero() {
		row("Updated", p.UpdatedAt.Format("2006-01-02 15:04"))
	}
	if len(p.Tags) > 0 {
		row("Tags", strings.Join(p.Tags, ", "))
	}
	// Which ref this package is pinned to locally. It is per package: the
	// header's method is only the default a fresh install starts from.
	//
	// A package that is not installed shows what its next install would use,
	// which is the only place the choice made with m is visible before the
	// confirmation appears.
	if entry.installed != nil {
		row("Install method", installedRefMethod(entry))
	} else if method != "" {
		row("Installs from", fmt.Sprintf("%s: %s", method, p.Ref(method)))
	}

	b.WriteString("\n")
	b.WriteString(renderStatus(entry, s))

	// What the machine needs before any of the steps below can run. Above the
	// steps rather than below them: this is what to read first when a build
	// fails, and the command is the answer to what to do about it.
	if req := entry.requirements(method); !req.Empty() {
		b.WriteString("\n" + s.DetailTitle.Render("Requirements") + "\n")
		for _, tool := range req.Toolchain {
			b.WriteString(s.Muted.Render("  "+s.Icons.Bullet+" ") + tool + "\n")
		}
		if cmd := pkg.ZypperCommand(req.OpenSUSE); cmd != "" {
			b.WriteString(wrap.Render(s.Muted.Render("  openSUSE — select with v and copy with y:")) + "\n")
			b.WriteString(wrap.Render("  "+cmd) + "\n")
		}
	}

	if len(p.Install.Steps) > 0 {
		b.WriteString("\n" + s.DetailTitle.Render("Build steps") + "\n")
		for i, step := range p.Install.Steps {
			b.WriteString(s.Muted.Render(fmt.Sprintf("  %d. ", i+1)))
			b.WriteString(wrap.Render(step) + "\n")
		}
	}

	if len(p.Install.Binaries) > 0 {
		b.WriteString("\n" + s.DetailTitle.Render("Binaries") + "\n")
		for _, bin := range p.Install.Binaries {
			b.WriteString(s.Muted.Render("  "+s.Icons.Bullet+" ") + bin + "\n")
		}
	}

	// Resources are the one thing an install puts outside bin/, configs/ and man/,
	// so where they land is worth showing rather than leaving to be discovered.
	if len(p.Install.Resources) > 0 {
		b.WriteString("\n" + s.DetailTitle.Render("Resources") + "\n")
		for _, res := range p.Install.Resources {
			b.WriteString(s.Muted.Render("  "+s.Icons.Bullet+" ") + res.Target + "\n")
		}
	}

	if len(p.Install.AdditionalConfig) > 0 {
		b.WriteString("\n" + s.DetailTitle.Render("Config files") + "\n")
		for _, ac := range p.Install.AdditionalConfig {
			b.WriteString(s.Muted.Render("  "+s.Icons.Bullet+" ") + ac.Filename + "\n")
		}
	}

	return b.String()
}

// installedRefMethod is the method a package is pinned to on disk, defaulting
// to version for a manifest written before the field existed.
func installedRefMethod(entry packageItem) string {
	if entry.installed == nil || entry.installed.InstallMethod == "" {
		return pkg.MethodVersion
	}
	return entry.installed.InstallMethod
}

// renderStatus describes how the package relates to the local installation.
func renderStatus(entry packageItem, s Styles) string {
	if entry.installed == nil {
		return s.Muted.Render("Not installed") + "\n"
	}

	method := installedRefMethod(entry)
	installedRef := entry.installed.Ref(method)
	if method == pkg.MethodCommit {
		installedRef = shortCommit(installedRef)
	}

	// A manifest without its artifacts is not an install. Saying so here is the
	// difference between "why does this not run" and one glance.
	if entry.broken {
		return s.Err.Render("Broken") +
			s.Muted.Render(fmt.Sprintf("  recorded as %s: %s, but its files are gone", method, installedRef)) + "\n" +
			s.Muted.Render("  install again to rebuild it") + "\n"
	}

	line := s.OK.Render("Installed") +
		s.Muted.Render(fmt.Sprintf("  %s: %s", method, installedRef)) + "\n"

	if entry.hasUpdate() {
		available := entry.pkg.Ref(method)
		if method == pkg.MethodCommit {
			available = shortCommit(available)
		}
		line += s.BadgeUpdate.Render("Update available") +
			s.Muted.Render("  → "+available) + "\n"
	}

	return line
}

// shortCommit trims a full SHA down to something readable.
func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
