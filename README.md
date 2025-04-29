# mgbfc

Brainfuck compiler implemented in Go. Generating ELF executable only for x86-64.

## Dependencies

- [go](https://go.dev/) - go compiler
- as - GNU assembler, used as backend
- ld - GNU linker, used for generating ELF64 executable

## Quick Start

```shell
$ go build mgbfc.go
$ ./mgbfc -o hello examples/hello_world.bf
$ ./hello
```

Also you can see usage documentation via `-h` flag

```shell
$ mgbfc -h
```

## Vim compiler plugin

Also this repo contains `mgbfc.vim` compiler plugin. To use it you should put this file in `vim-config-dir/compiler` (`$HOME/.vim/compiler` or `$HOME/.config/vim/compiler`). This will work also with neovim. After that you can use it via `:compiler mgbfc` and do `:make` with quickfix-list support for compilation errors.

See `:help compiler` for additional information about compiler plugins in vim.
