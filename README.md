# Clipack

Clipack is a package management tool that allows you to easily install, update, and remove packages on your system.

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

### Updating Packages

To update a package, use the `update` command:
```sh
clipack update [package-name]
```
You can force refresh the package registry cache by using the `-f` flag:
```sh
clipack update [package-name] -f
```

### Removing Packages

To remove a package, use the `remove` command:
```sh
clipack remove [package-name]
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
