{ pkgs, beehive, beehived }:

pkgs.testers.nixosTest {
  name = "beehive-multi-repo";

  nodes.machine = { pkgs, ... }: {
    imports = [ ../../module.nix ];

    _module.args.inputs = { };

    environment.systemPackages = [ pkgs.curl ];

    services.beehived = {
      enable = true;
      package = beehived;
      # Unused once repos.yaml resolves a non-empty registry — required by
      # the module's option type regardless.
      repoRoot = "/var/lib/beehived-test/alpha";
    };

    systemd.services.beehived = {
      serviceConfig.StateDirectory = "beehived-test";
      environment.BEEHIVE_CONFIG_DIR = "/var/lib/beehived-test/config";
      preStart = ''
        set -e
        base=/var/lib/beehived-test
        ${beehive}/bin/beehive init "$base/alpha"
        ${beehive}/bin/beehive init "$base/beta"
        mkdir -p "$base/alpha/submodules/flux" "$base/beta/submodules/other"
        printf '<!-- Beehive-ROI: deadbeef -->\n# Plan\n' > "$base/alpha/submodules/flux/PLAN.md"
        printf '<!-- Beehive-ROI: deadbeef -->\n# Plan\n' > "$base/beta/submodules/other/PLAN.md"

        mkdir -p "$base/config"
        {
          printf 'repos:\n'
          printf '  - name: alpha\n'
          printf '    root: %s\n' "$base/alpha"
          printf '    gpg_home: %s\n' "$base/alpha/gnupg"
          printf '    gpg_recipient: alpha@example.com\n'
          printf '  - name: beta\n'
          printf '    root: %s\n' "$base/beta"
          printf '    gpg_home: %s\n' "$base/beta/gnupg"
          printf '    gpg_recipient: beta@example.com\n'
        } > "$base/config/repos.yaml"
      '';
    };
  };

  testScript = ''
    machine.start()
    machine.wait_for_unit("beehived.service")
    machine.wait_for_open_port(8955)

    # Default (no cookie) active repo is the alphabetically-first name: alpha.
    alpha = machine.succeed("curl -sf http://127.0.0.1:8955/dashboard.json")
    assert "flux" in alpha, alpha
    assert "other" not in alpha, alpha

    machine.succeed(
        "curl -sf -c /tmp/jar -o /dev/null -X POST http://127.0.0.1:8955/repo/beta"
    )
    beta = machine.succeed("curl -sf -b /tmp/jar http://127.0.0.1:8955/dashboard.json")
    assert "other" in beta, beta
    assert "flux" not in beta, beta
  '';
}
