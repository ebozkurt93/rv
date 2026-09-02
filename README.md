# rv

A terminal UI for reviewing a `git diff`, with comments a coding agent can read and reply to from the same shell.

You review your working-tree changes in the TUI and leave comments anchored to specific lines. A coding agent (Claude Code, or anything else you point at it) picks those up with a couple of CLI subcommands, fixes the code, and replies — no server, no shared account, no copy-pasting feedback back and forth. Everything lives in one JSON file per repo checkout under `~/.cache/rv/`, guarded by a file lock so the TUI and an agent's CLI calls can run at the same time.

This entire codebase is AI-written. It's been in daily personal use for a while and works well for that, but it comes with no guarantees — use it accordingly, and expect to read the code (or file an issue) if something breaks for your setup.

## Install

With [Nix](https://nixos.org) (flakes enabled), try it without installing anything:

```sh
nix run github:ebozkurt93/rv
```

...or install it to your profile:

```sh
nix profile install github:ebozkurt93/rv
```

...or add it as a flake input, e.g. in a NixOS/home-manager config:

```nix
{
  inputs.rv.url = "github:ebozkurt93/rv";

  outputs = { self, nixpkgs, rv, ... }: {
    # NixOS: environment.systemPackages = [ rv.packages.${system}.default ];
    # home-manager: home.packages = [ rv.packages.${system}.default ];
  };
}
```

Or with Go 1.25+:

```sh
go install github.com/ebozkurt93/rv@latest
```

## Usage

```sh
rv                  # working tree vs HEAD (the default)
rv HEAD~3           # vs an older commit
rv main..feature    # a branch range
rv --staged         # staged changes only
```

Any argument that isn't a known subcommand is passed straight through to `git diff` — `rv` doesn't reimplement revision syntax.

Move around with `j`/`k`, `{`/`}` for hunks, `tab`/`shift+tab` for files. Press `c` to leave a comment on the line under the cursor, `d` to delete one, `r` to toggle resolved. Press `?` inside the TUI for the full keybinding reference.

## Agent handoff

Tell your agent something like "check rv comments" or "address the rv feedback." It reads your comments, fixes the code, and replies — you'll see the reply in the TUI within about a second, no reopening required.

```sh
rv comment list --unresolved --json
rv comment reply <id> "fixed, see foo.go:42" --resolve
```

The full CLI reference (`rv comment list/resolve/reply`, `rv session get`) is in `rv skill print` — that's also the canonical instructions doc if you want to wire an agent up yourself; it's designed to be handed to one directly.

## Configuration

Keybindings can be remapped in `~/.config/rv/config` (or `$XDG_CONFIG_HOME/rv/config`):

```
# one binding per line, comma-separated for multiple keys
add_comment = c
next_comment = n, down
```

Run `rv --help` to see everything else, or `?` inside the TUI for the full keybinding list with whatever you've remapped.

## Development

```sh
make build   # go build -o rv .
make test    # go test ./...
make fmt     # gofmt -w *.go
```

Everything runs inside a Nix dev shell (`.envrc`/`flake.nix`) if you have direnv, but plain `go build`/`go test` work too as long as you have Go 1.25+.

## License

MIT — see [LICENSE](LICENSE).
