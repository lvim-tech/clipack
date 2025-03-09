# Clipack

Clipack is a package management tool that allows you to easily install, update, and remove packages on your system.

**Current Version: Beta v0.0.70**

## Installation

To install Clipack, follow these steps:

1. Clone the repository:

    ```sh
    git clone https://github.com/lvim-tech/clipack.git
    cd clipack
    ```

2. Build the program:

    ```sh
    go build -o bin/clipack
    ```

3. Move the executable to your executable files directory:
    ```sh
    mv bin/clipack /usr/local/bin/
    ```

## Usage

### Installing Packages

To install a package, use the `install` command:

```sh
clipack install [package-name]
```

You can specify the installation method using the `--install-method` flag. Possible values are `version` and `commit`:

```sh
clipack install [package-name] --install-method=version
```

If the `--install-method` flag is not specified, the value from the configuration file will be used.

You can force refresh the package registry cache by using the `--force-refresh` flag:

```sh
clipack install [package-name] --force-refresh
```

### Updating Packages

To update a package, use the `update` command:

```sh
clipack update [package-name]
```

You can specify the installation method using the `--install-method` flag. Possible values are `version` and `commit`:

```sh
clipack update [package-name] --install-method=version
```

If the `--install-method` flag is not specified, the value from the configuration file will be used.

You can force refresh the package registry cache by using the `--force-refresh` flag:

```sh
clipack update [package-name] --force-refresh
```

### Removing Packages

To remove a package, use the `remove` command:

```sh
clipack remove [package-name]
```

### Previewing Packages

To preview the available packages in the registry, use the `preview` command:

```sh
clipack preview
```

You can preview a specific package by providing its name:

```sh
clipack preview [package-name]
```

You can force refresh the package registry cache by using the `--force-refresh` flag:

```sh
clipack preview --force-refresh
```

### Listing Packages

To list available packages, use the `list` command:

```sh
clipack list
```

You can force refresh the package registry cache by using the `--force-refresh` flag:

```sh
clipack list --force-refresh
```

### Example Package YAML Configuration File

Here is an example of a package configuration:

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
    - ls
    - colors
    - themes
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
    additional-config:
        - filename: config.sh
          content: |
              #!/usr/bin/env bash

              BASE_PATH=$(grep 'base:' $HOME/.config/clipack/config.yaml | sed 's/.*base: //')

              if [ -z "$THEME" ]; then
                  THEME="LvimDark"
              fi

              export LS_COLORS="$($BASE_PATH/bin/vivid generate $BASE_PATH/configs/vivid/$THEME.yml)"
```

## Registry

Clipack uses package registry files from [Clipack Registry](https://github.com/lvim-tech/clipack-registry). You can specify a different registry URL in the configuration file if needed.

## License

This project is licensed under the BSD 3-Clause License - see the [LICENSE](LICENSE) file for details.
