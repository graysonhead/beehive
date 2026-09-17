{
  description = "beehive — deterministic git-native workflow tooling and its LLM-agent runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    flake-compat.url = "github:edolstra/flake-compat";
  };

  outputs = { self, nixpkgs, flake-utils, flake-compat }:
    (flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = import nixpkgs { inherit system; };

          version = "0.0.0-dev";

          mkBeehiveBin = name: pkgs.buildGoModule {
            pname = name;
            inherit version;
            src = ./.;
            subPackages = [ "cmd/${name}" ];
            vendorHash = "sha256-kNXKWxpXaBbDu1++NO4U1sow8wRY59Ewgye/nT97rC4=";
            env.CGO_ENABLED = "0";
            ldflags = [ "-s" "-w" ];
            # go test needs a real `git` on PATH (repo/worktree tests shell out to
            # it); CI already runs `go test -race ./...`, so skip tests in the
            # Nix build rather than wiring nativeCheckInputs for a redundant run.
            doCheck = false;
          };
        in
        rec {
          packages = {
            beehive = mkBeehiveBin "beehive";
            beehived = mkBeehiveBin "beehived";
            honeybee = mkBeehiveBin "honeybee";
            default = packages.beehive;
          };

          apps = {
            beehive = flake-utils.lib.mkApp { drv = packages.beehive; };
            beehived = flake-utils.lib.mkApp { drv = packages.beehived; };
            honeybee = flake-utils.lib.mkApp { drv = packages.honeybee; };
            default = apps.beehive;
          };

          nixosModules = rec {
            beehived = import ./module.nix;
            default = beehived;
          };

          devShells.default = pkgs.mkShell {
            nativeBuildInputs = with pkgs; [
              go
              gopls
              golangci-lint
              git
              gnupg
            ];
          };
        }
      ))
    // {
      checks.x86_64-linux = {
        beehived-integration = import ./nix/tests/beehived-integration.nix {
          pkgs = nixpkgs.legacyPackages.x86_64-linux;
          beehive = self.packages.x86_64-linux.beehive;
          beehived = self.packages.x86_64-linux.beehived;
        };
        beehive-task-daemon = import ./nix/tests/beehive-task-daemon.nix {
          pkgs = nixpkgs.legacyPackages.x86_64-linux;
          beehive = self.packages.x86_64-linux.beehive;
          beehived = self.packages.x86_64-linux.beehived;
        };
        beehive-secrets = import ./nix/tests/beehive-secrets.nix {
          pkgs = nixpkgs.legacyPackages.x86_64-linux;
          beehive = self.packages.x86_64-linux.beehive;
        };
        beehive-multi-repo = import ./nix/tests/beehive-multi-repo.nix {
          pkgs = nixpkgs.legacyPackages.x86_64-linux;
          beehive = self.packages.x86_64-linux.beehive;
          beehived = self.packages.x86_64-linux.beehived;
        };
        beehive-bad-registry = import ./nix/tests/beehive-bad-registry.nix {
          pkgs = nixpkgs.legacyPackages.x86_64-linux;
          beehived = self.packages.x86_64-linux.beehived;
        };
        beehive-honeybee-schedule = import ./nix/tests/beehive-honeybee-schedule.nix {
          pkgs = nixpkgs.legacyPackages.x86_64-linux;
          beehive = self.packages.x86_64-linux.beehive;
          honeybee = self.packages.x86_64-linux.honeybee;
        };
      };
    };
}
