# flake.nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_26  # Or go (for latest stable)
            gopls
            golangci-lint
          ];

          shellHook = ''
            echo "Using Go $(go version)"
            export GOPATH="$PWD/.gopath"
            mkdir -p "$GOPATH"
          '';
        };
      });
}   
