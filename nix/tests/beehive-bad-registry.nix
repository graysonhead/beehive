{ pkgs, beehived }:

pkgs.testers.nixosTest {
  name = "beehive-bad-registry";

  nodes.machine = { pkgs, ... }: {
    imports = [ ../../module.nix ];

    _module.args.inputs = { };

    services.beehived = {
      enable = true;
      package = beehived;
      # Unused: startup fails resolving the (invalid) registry before this
      # is ever touched.
      repoRoot = "/var/lib/beehived-test/repo";
    };

    systemd.services.beehived = {
      serviceConfig.StateDirectory = "beehived-test";
      environment.BEEHIVE_CONFIG_DIR = "/var/lib/beehived-test/config";
      preStart = ''
        set -e
        mkdir -p /var/lib/beehived-test/config
        {
          printf 'repos:\n'
          printf '  - name: dup\n'
          printf '    root: /var/lib/beehived-test/repo-a\n'
          printf '    gpg_home: /var/lib/beehived-test/repo-a/gnupg\n'
          printf '    gpg_recipient: a@example.com\n'
          printf '  - name: dup\n'
          printf '    root: /var/lib/beehived-test/repo-b\n'
          printf '    gpg_home: /var/lib/beehived-test/repo-b/gnupg\n'
          printf '    gpg_recipient: b@example.com\n'
        } > /var/lib/beehived-test/config/repos.yaml
      '';
    };
  };

  testScript = ''
    machine.start()
    machine.wait_for_unit("multi-user.target")

    # The duplicate `name: dup` above fails Registry.Validate(), which
    # beehived's main.go treats as fatal (log.Fatalf) rather than degrading
    # or falling back silently — confirm that failure is actually observable
    # (failed unit + the specific registry error in the journal), not masked.
    machine.fail("systemctl is-active --quiet beehived.service")
    machine.succeed("journalctl -u beehived.service | grep -q 'registry:'")
  '';
}
