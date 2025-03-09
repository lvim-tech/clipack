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
    go build -o clipack
    ```

3. Move the executable to your executable files directory:
    ```sh
    mv clipack /usr/local/bin/
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
steps:
    - git clone --branch v0.20.22 --single-branch https://github.com/eza-community/eza.git .
    - cargo build --release
    - mkdir -p man/man1 man/man5
    - pandoc man/eza.1.md -s -t man -o man/man1/eza.1
    - pandoc man/eza_colors-explanation.5.md -s -t man -o man/man5/eza_colors-explanation.5
    - pandoc man/eza_colors.5.md -s -t man -o man/man5/eza_colors.5

binaries:
    - target/release/eza

man:
    - man/man1/eza.1
    - man/man5/eza_colors-explanation.5
    - man/man5/eza_colors.5
```

## Registry

Clipack uses package registry files from [Clipack Registry](https://github.com/lvim-tech/clipack-registry).

## License

This project is licensed under the BSD 3-Clause License - see the [LICENSE](LICENSE) file for details.
