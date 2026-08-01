// Command clipack builds CLI tools from source and manages their binaries,
// man pages and configuration files.
//
// Packages are described by YAML files in a git registry. Running clipack with
// no arguments opens an interactive interface; the subcommands provide the same
// operations non-interactively for use in scripts.
//
// See the cmd package for the command line, tui for the interface, and pkg for
// the registry access and the installer both of them drive.
package main

import "github.com/lvim-tech/clipack/cmd"

func main() {
	cmd.Execute()
}
