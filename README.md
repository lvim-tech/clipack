# Clipack

Clipack builds CLI tools from source and manages their binaries, man pages and
configuration files. Packages are described by YAML files in a git registry, so
adding a tool means adding a file — no plugin, no scripting.

It ships with a terminal interface built on [Bubble Tea](https://github.com/charmbracelet/bubbletea):
browse the registry, filter it, and watch a build stream live. Every operation is
also available non-interactively for scripts.

**Current version: Beta v0.0.72**

---

## Install

```sh
git clone https://github.com/lvim-tech/clipack.git
cd clipack
go build -o clipack .
sudo mv clipack /usr/local/bin/
```

Requires Go 1.24 or newer.

On first run clipack asks where to keep its files and offers to add `bin/` and
`man/` to your shell rc file. The default layout is:

```
~/clipack/
├── bin/        installed binaries
├── configs/    per-package configuration + the install manifest
├── build/      source trees (removed after install unless cleanup_build: false)
├── man/        man pages, split into man1/, man5/, …
└── registry/   the registry cache
```

The configuration itself lives at `~/.config/clipack/config.yaml`.

---

## The interface

Run `clipack` with no arguments:

```
  clipack    21 packages · 8 installed · 13 updates  ·  method: version
  All    Installed    Updates (13)
 ┌───────────────────────────────────┐┌──────────────────────────────────────────┐
 │ ▌ atuin v18.6.1 ↑ update          ││ atuin  v18.6.1                           │
 │     Atuin - Magical shell history ││                                          │
 │                                   ││ Description   Atuin - Magical shell …    │
 │   bat v0.25.0 ↑ update            ││ Category      cli                        │
 │     A cat(1) clone with syntax …  ││ Maintainer    Atuin Team                 │
 │                                   ││ License       MIT                        │
 │   cronboard 2.1.0 ✓ installed     ││ Homepage      https://github.com/…       │
 │     Fast TUI app launcher and     ││                                          │
 │     fuzzy finder for GNU/Linux    ││ Installed  commit: 8c18b2e960bd          │
 │                                   ││ Update available  → a272ea753a34         │
 │   ● ○ ○ ○                         ││                                          │
 └───────────────────────────────────┘└──────────────────────────────────────────┘
```

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | move within the focused pane |
| `→`/`l` | focus the details pane |
| `←`/`h`, `esc` | focus the list |
| `tab`, `shift+tab` | switch between **All**, **Installed** and **Updates** |
| `i` or `enter` | install the selected package |
| `u` | update it |
| `x` | remove it |
| | *a package offers install, or update and remove — never both* |
| `m` | toggle between the `version` and `commit` install method |
| `r` | refresh the registry cache |
| `p` | add `bin/` to the current shell's startup file — offered only while that shell cannot find it |
| `/` | filter by name, description, category or tag |
| `pgup` / `pgdn` | page the focused pane |
| `?` | expanded help |
| `q`, `ctrl+c` | quit |

The focused pane is the one the movement keys drive; it is drawn with the accent
border. `i`, `u` and `x` always act on the selected package, so they work from
either pane.

The help line at the bottom follows the cursor: it offers `i` for a package that
is not installed, and `u` and `x` for one that is. Reinstalling over an existing
install is refused — it would leave the previous version's binaries behind, since
only `update` reads the old manifest and knows what to clean up. Use `update` to
upgrade, or `remove` and then `install` for a clean slate.

### Selecting and copying text

The details pane has a text cursor and vi's visual mode, so a homepage, a commit
or a whole build step can be copied without reaching for the mouse:

| Key | Action |
|---|---|
| `h` `j` `k` `l`, arrows | move the cursor (`h` at column 0 returns to the list) |
| `0`, `$` | start / end of line |
| `g`, `G` | first / last line |
| `ctrl+u`, `ctrl+d` | half a page |
| `v` | character-wise selection |
| `V` | line-wise selection |
| `y` | copy the selection — with none, copies the cursor line |
| `esc` | cancel the selection; again to return to the list |

Copying uses the system clipboard (`wl-copy`, `xclip`, `xsel`, `pbcopy`). When
none is available — a bare SSH session — it falls back to OSC 52 and asks your
terminal emulator to do it instead.

Every install, update and remove is confirmed first, then runs on the **run**
screen, where the build's output is streamed line by line with a step counter.

---

## Commands

Each command works without the interface, so clipack can be scripted.

### install

```sh
clipack install bat                 # one package
clipack install bat fzf zoxide      # several
clipack install bat -y              # no confirmation prompt
clipack install bat -m commit       # pin to the registry's commit
clipack install bat -f              # refresh the registry cache first
clipack install                     # no arguments → opens the interface
```

`-m/--install-method` accepts `version` (checks out the tag in `version:`) or
`commit` (checks out the sha in `commit:`). Without the flag the
`options.install_method` value from the configuration is used.

### update

```sh
clipack update                      # list what is out of date
clipack update --all                # update everything, confirming each
clipack update bat fzf              # update named packages
clipack update --all -y             # unattended
```

Each package is updated using the method it was installed with, so a package
pinned to a commit is not silently moved onto a version tag.

### remove

```sh
clipack remove bat
clipack remove bat fzf -y
clipack rm bat                      # alias
```

Removal is driven by the manifest written at install time
(`configs/<name>/package.yaml`), which is the only accurate record of what was
put on disk — the registry entry may have changed since.

### list

```sh
clipack list                        # every package, as a table
clipack list --installed            # only what is installed
clipack list --updates              # only what is out of date
clipack list -f                     # refresh the cache first
```

```
NAME        VERSION  STATUS     CATEGORY       DESCRIPTION
atuin       v18.6.1  update     cli            Atuin - Magical shell history
bat         v0.25.0  update     cli            A cat(1) clone with syntax highlighting
cronboard   2.1.0    installed  cli            Fast TUI app launcher and fuzzy finder
```

### preview

```sh
clipack preview bat                 # the full registry record as YAML
clipack preview bat -f
```

### add-executables-path

```sh
clipack add-executables-path        # alias: clipack path
```

Appends `bin/` and `man/` to the startup file of the shell you are running it
from, and only that one. bash, zsh, ksh, mksh, yash, dash, fish, csh, tcsh,
nushell, elvish, xonsh and PowerShell are recognised, each written in its own
syntax. Safe to run repeatedly — it skips the write if the path is already
there.

The shell is identified from the process that launched clipack rather than from
`$SHELL`, which names your *login* shell and does not change when you start a
different one. Set `CLIPACK_SHELL` to override the guess.

Run it again from your other shells: each keeps its own startup file, so each
needs the line. The interface offers the same thing — a warning and a `p` key —
whenever the shell it is running under cannot find `bin/`.

### theme

```sh
clipack theme                       # list every theme, mark the active one
clipack theme LvimNord_dark         # switch
clipack theme --colors              # show the resolved palette
```

See [Theming](#theming) below.

### update-config

```sh
clipack update-config
```

Repoints the installation directory. The `registry` and `options` sections of
your existing configuration are preserved, so a token or a custom install method
survives.

---

## Configuration

`~/.config/clipack/config.yaml`:

```yaml
registry:
    url: https://github.com/lvim-tech/clipack-registry.git
    registryRepoURL: https://api.github.com/repos/lvim-tech/clipack-registry/contents
    branch: main
    update_interval: 24h
    # token: ghp_…            # only needed for a private registry

paths:
    base: /home/user/clipack
    registry: /home/user/clipack/registry
    bin: /home/user/clipack/bin
    configs: /home/user/clipack/configs
    build: /home/user/clipack/build
    man: /home/user/clipack/man

options:
    auto_symlink: true
    backup_configs: true
    cleanup_build: true # remove the source tree after a successful install
    install_method: version # or: commit

theme:
    name: default
```

| Key | Meaning |
|---|---|
| `registry.url` | The registry repository. Owner and name are derived from it. |
| `registry.branch` | Branch to read the registry from. |
| `registry.update_interval` | How long a cached registry stays fresh. |
| `registry.token` | Only required for a private registry. An invalid token is ignored — clipack falls back to anonymous access. |
| `options.install_method` | Default for `install`; per-command via `-m`. |
| `options.cleanup_build` | Whether the build tree is deleted after installing. |

All paths must be absolute.

---

## Theming

Colours, borders and glyphs all come from the `theme` section of
`config.yaml`. A theme is a **base** — either built in or a file — plus optional
overrides.

```sh
clipack theme                       # what is available, and what is active
clipack theme LvimNord_dark         # switch
clipack theme --colors              # the resolved palette
```

### Built-in themes

| Name | What it is |
|---|---|
| `default` | Adapts to a light or dark terminal background. |
| `mono` | No colour at all — emphasis comes from weight and dimming, glyphs are ASCII, borders are square. For a serial console, a recording, or a palette clipack cannot know. |

### Installing generated themes

[lvim-colorscheme](https://github.com/lvim-tech/lvim-colorscheme) generates a
clipack theme for each of its 48 styles. Copy them in once:

```sh
mkdir -p ~/.config/clipack/themes
cp ~/path/to/lvim-colorscheme/extras/clipack/*.yaml ~/.config/clipack/themes/
clipack theme LvimNord_dark
```

Any `.yaml` file in `~/.config/clipack/themes` becomes selectable by its
filename, so hand-written themes work the same way.

### Writing a theme

```yaml
name: LvimNord_dark

# normal | rounded | thick | double | hidden | none
border: normal

# unicode | ascii
icons: unicode

colors:
    accent: "#a58aa0" # selection cursor, focused border, title badge
    accent_alt: "#8097af" # section headings, active tab, step markers
    text: "#b3bac6" # default foreground
    muted: "#677185" # descriptions, labels, hints, build output
    subtle: "#3c475a" # unfocused pane borders
    success: "#97ab86" # installed packages, completed operations
    warning: "#cbae72" # available updates, non-fatal problems
    error: "#af7177" # failures
    title_fg: "#232831" # text of the title badge, drawn on accent
```

A colour is a hex value (`#b48ead` or `#abc`) or an ANSI palette index (`0`–`255`).
To follow the terminal's background, give both variants:

```yaml
colors:
    text: { light: "#1c1c1c", dark: "#e5e9f0" }
```

Leaving a colour out is fine — it falls back to the base theme. Leaving all of
them out means clipack uses the terminal's own colours for that role.

### Overriding

Anything in `config.yaml` wins over the theme, so a single tweak does not mean
copying the palette:

```yaml
theme:
    name: LvimNord_dark
    border: double # this theme, but square-ish borders
    colors:
        accent: "#ff0000" # and a different cursor colour
```

`clipack theme <name>` tells you when such an override is still in place, since
that is the usual reason switching appears to do nothing.

A theme that cannot be resolved — a typo, or a file that was deleted — does not
stop clipack. The interface falls back to `default` and shows the reason in its
status line.

---

## Registry

Packages come from [clipack-registry](https://github.com/lvim-tech/clipack-registry).
`index.yaml` lists the package files; the directory a file sits in becomes its
category:

```yaml
packages:
    - packages/cli/bat.yaml
    - packages/file_managers/yazi.yaml
```

A package file:

```yaml
name: vivid
version: v0.10.1
commit: 782907221045fbcd4df62b2061f92fcaf6b637aa
description: A themeable LS_COLORS generator with a rich filetype database.
homepage: https://github.com/sharkdp/vivid
license: MIT
maintainer: sharkdp
updated_at: 2025-02-24T13:45:00Z
tags:
    - cli
    - colors
install:
    source:
        type: git
        url: https://github.com/sharkdp/vivid.git
        ref: main
    steps:
        - git clone https://github.com/sharkdp/vivid.git .
        - cargo build --release
    binaries:
        - target/release/vivid
    man:
        - man/man1/vivid.1
    additional-config:
        - filename: config.sh
          content: |
              #!/usr/bin/env bash
              export LS_COLORS="$(vivid generate lvim)"
        - filename: themes/lvim.yml
          content: https://raw.githubusercontent.com/…/lvim.yml
```

**Fields**

| Field | Meaning |
|---|---|
| `version` / `commit` | The two refs a package can be pinned to. |
| `install.source.url` | Preferred source of the clone URL. |
| `install.steps` | Shell commands, run in order inside the build directory. |
| `install.binaries` | Paths, relative to the build directory, copied into `bin/`. |
| `install.resources` | Directory trees a program needs beside its binary, as `source` (relative to the build directory) and `target` (relative to `base`). Recorded in the manifest, so uninstalling removes them. See below. |
| `install.desktop` | Menu entries a graphical program ships, as `source` (a `.desktop` file relative to the build directory), an optional `icon` and an optional `name`. Installed into the user's application directory. See below. |
| `install.configs` | Files copied from the build tree into `configs/<name>/`. |
| `install.man` | Man pages; the extension picks the section (`.1` → `man1/`). |
| `install.additional-config` | Files written into `configs/<name>/`. A value starting with `http://` or `https://` is downloaded; anything else is used literally. `.sh` files are made executable. |
| `install.environment` | Extra environment variables for the build. |
| `post-install.scripts` | Scripts written into `bin/` and made executable. |

**Resources**

Not every program is a single self-contained executable. kitty's launcher is
compiled with `KITTY_LIB_PATH="../lib/kitty"` and loads its Python package from
there; ghostty looks for its terminfo and shell integration in
`../share/ghostty`. Both resolve that path against the location of the binary,
so with binaries in `bin/` the trees have to land in `lib/` and `share/`:

```yaml
    binaries:
        - linux-package/bin/kitty
    resources:
        - source: linux-package/lib/kitty
          target: lib/kitty
```

Declaring them here rather than copying them from an install step is what makes
them removable — they go into the manifest, so `remove` deletes them and prunes
the directories that emptied. An update replaces the tree instead of merging
into it, so a file the previous version shipped cannot outlive it.

`target` is validated before anything is written or deleted. It has to stay
inside `base`, may not touch `bin/`, `configs/`, `build/`, `man/` or
`registry/`, and may not be a top-level directory: `lib/kitty` belongs to one
package and removing it is safe, whereas `lib` is shared and the first uninstall
would empty it for everyone.

**Desktop entries**

A graphical program installed by clipack is not in any menu, because nothing put
a `.desktop` file where launchers look. Declaring the one the package already
builds fixes that:

```yaml
    desktop:
        - source: linux-package/share/applications/kitty.desktop
          icon: linux-package/share/icons/hicolor/256x256/apps/kitty.png
```

The entry is installed into `$XDG_DATA_HOME/applications` (`~/.local/share/applications`
by default) as `clipack-<package>-<file>.desktop`. No root is needed, and nothing
in a system directory is touched or overwritten.

Three things are rewritten on the way in:

- **`Exec` and `TryExec`** are repointed at the binary in clipack's `bin/`. This
  is the part that matters. A shipped entry says `Exec=kitty` and leaves the
  choice to `PATH`, so on a machine that also has the distribution's package the
  menu entry would list the clipack build and launch the other one. Entries in
  `[Desktop Action …]` groups are rewritten too. A program that is not in `bin/`
  is left alone.
- **`Name`** gets ` (clipack)` appended, so the two entries are distinguishable
  in the launcher. Localised `Name[…]` variants are suffixed as well; an action's
  name is not. Set `name:` to replace it outright instead.
- **`Icon`** is repointed at the installed icon when one is declared. Without
  `icon:` the entry falls back to the icon theme, which has a matching icon only
  when a system package supplied one.

Entries and icons go into the manifest, so `remove` deletes them. Removal can
only reach files clipack wrote: the installed name is derived from the package
name and prefixed, never taken from the manifest as-is.

A failure here is a warning rather than an error. A program that is installed
and runnable but missing from the menu is still a working install.

**How steps run**

Each step is executed through `sh -c` inside the build directory, so quoting,
pipes and `&&` behave as written. The `git clone` step is rewritten to honour the
selected method:

- `version` → `git clone --branch <version> --single-branch --depth 1 <url> .`
- `commit` → `git clone <url> .` followed by `git checkout <commit>`

so the ref you asked for is the ref you get.

---

## How it works

Clipack fetches the whole registry as a single repository tarball from
`codeload.github.com`, which is one HTTP request for the entire package set. If
that is unavailable it falls back to one raw request per file, eight at a time.
The result is cached in `registry/packages_cache.gob` for
`registry.update_interval`. A partial fetch is shown but never cached, so a
network hiccup cannot make packages disappear for a day.

Installing a package builds it in `build/<name>/`, copies the declared artifacts
into place, and writes the resolved package definition to
`configs/<name>/package.yaml`. That manifest is what `list`, `update` and
`remove` read to know what is installed and what it was pinned to.

---

## Development

```sh
go build ./...
go vet ./...
gofmt -l .
go test ./... -race -cover
go build -o bin/clipack . && ./bin/clipack
```

| Package | Tests | Coverage |
|---|---:|---:|
| `pkg` | 88 | 84.6 % |
| `tui` | 164 | 85.9 % |
| `cmd` | 85 | 82.2 % |
| `cnfg` | 76 | 87.7 % |
| `utils` | 16 | 95.2 % |

The tests are hermetic — no network and no access to your real installation.
Registry fetches run against an in-process `httptest` server, every test points
`HOME` at a temporary directory, the Bubble Tea model is driven by feeding it
messages rather than by opening a terminal, and the commands resolve packages
from a pre-seeded cache so an install builds for real from shell built-ins.

Layout:

| Directory | Contents |
|---|---|
| `cmd/` | Cobra commands. Argument parsing only. |
| `tui/` | The Bubble Tea interface. |
| `pkg/` | Package types, registry access, cache, installer. |
| `cnfg/` | Configuration loading and the shell rc integration. |
| `utils/` | Small shared helpers. |
| `scripts/` | Maintenance scripts for the registry repository. |

`pkg.Installer` reports progress as `pkg.Event` values through a callback rather
than printing, which is why the CLI and the TUI can share one implementation.

---

## License

BSD 3-Clause — see [LICENSE](LICENSE).
