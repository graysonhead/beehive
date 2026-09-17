{ pkgs, beehive, beehived }:

pkgs.testers.nixosTest {
  name = "beehive-task-daemon";

  nodes.machine = { pkgs, ... }: {
    imports = [ ../../module.nix ];

    _module.args.inputs = { };

    environment.systemPackages = [ pkgs.curl ];

    services.beehived = {
      enable = true;
      package = beehived;
      repoRoot = "/var/lib/beehived-test/repo";
    };

    systemd.services.beehived = {
      serviceConfig.StateDirectory = "beehived-test";
      # Seed a repo with the `beehive` CLI, then create a task through it —
      # this proves beehived reads back exactly what the CLI wrote, not just
      # that both binaries build.
      preStart = ''
        set -e
        repo=/var/lib/beehived-test/repo
        ${beehive}/bin/beehive init "$repo"
        cd "$repo"
        git config user.email t@t.test
        git config user.name test
        mkdir -p submodules/widget
        printf '<!-- Beehive-ROI: deadbeef -->\n# Plan\n' > submodules/widget/PLAN.md
        ${beehive}/bin/beehive task add widget mytask --body "task card body" --doc "# Task Doc"
      '';
    };
  };

  testScript = ''
    machine.start()
    machine.wait_for_unit("beehived.service")
    machine.wait_for_open_port(8955)
    machine.succeed(
        "curl -sf http://127.0.0.1:8955/submodule/widget/plan.json | grep -q mytask"
    )
  '';
}
