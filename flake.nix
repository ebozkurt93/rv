{
  description = "Development environment for rv (local code review tool, Go)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    nixpkgs,
    flake-utils,
    ...
  }:
    flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = import nixpkgs {inherit system;};

        rv = pkgs.buildGoModule {
          pname = "rv";
          version = "0.1.0";
          src = ./.;
          vendorHash = null;
        };
      in {
        packages = {
          default = rv;
          rv = rv;
        };

        devShells = {
          default = pkgs.mkShell {
            buildInputs = [
              pkgs.go
              pkgs.gnumake
            ];
            # A version manager (e.g. mise) may export GOROOT/GOBIN pointing at a
            # different Go than the one this shell puts on PATH; unset them so
            # `go` always resolves its own toolchain paths.
            shellHook = ''
              unset GOROOT
              unset GOBIN
            '';
          };
        };

        apps = {
          default = flake-utils.lib.mkApp {drv = rv;};
        };
      }
    );
}
