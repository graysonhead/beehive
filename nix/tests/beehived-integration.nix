{ pkgs, beehive, beehived }:

pkgs.testers.nixosTest {
  name = "beehived-integration";

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
      # Idempotent: `beehive init` only `git init`s if `.git` is missing.
      preStart = "${beehive}/bin/beehive init /var/lib/beehived-test/repo";
      serviceConfig.StateDirectory = "beehived-test";
    };
  };

  testScript = ''
    machine.start()
    machine.wait_for_unit("beehived.service")

    # Regression check for module.nix's `path = [ pkgs.git pkgs.gnupg ];`:
    # beehived and the `beehive init` preStart both shell out to git, so
    # dropping that PATH wiring breaks startup rather than just misbehaving
    # at runtime.
    machine.succeed("systemctl show beehived.service -p Environment | grep -q git")

    machine.wait_for_open_port(8955)
    machine.succeed("curl -sf http://127.0.0.1:8955/prometheus/-/healthy | grep -q ok")
    machine.succeed("curl -sf http://127.0.0.1:8955/ >/dev/null")
  '';
}
