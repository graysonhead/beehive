{ pkgs, beehive }:

pkgs.testers.nixosTest {
  name = "beehive-secrets";

  nodes.machine = { pkgs, ... }: {
    environment.systemPackages = [ beehive pkgs.gnupg pkgs.git ];
  };

  testScript = ''
    machine.start()

    # A repo context (from `beehive init`) plus a from-scratch, no-passphrase
    # gpg keyring — same batch-gen-key recipe beehive's own secrets tests use.
    machine.succeed("${beehive}/bin/beehive init /root/repo")
    machine.succeed("install -d -m700 /root/beehive-config/gnupg")
    machine.succeed(
        "printf '%b' 'Key-Type: RSA\\nKey-Length: 2048\\nName-Real: bh\\n"
        "Name-Email: beehive-test@example.com\\nExpire-Date: 0\\n"
        "%no-protection\\n%commit\\n' | "
        "gpg --homedir /root/beehive-config/gnupg --batch --gen-key"
    )

    env = "BEEHIVE_CONFIG_DIR=/root/beehive-config"
    machine.succeed(
        f"cd /root/repo && {env} "
        "${beehive}/bin/beehive secret set foo bar --recipient beehive-test@example.com"
    )
    out = machine.succeed(
        f"cd /root/repo && {env} "
        "${beehive}/bin/beehive secret get foo --recipient beehive-test@example.com"
    )
    assert out.strip() == "bar", f"unexpected secret value: {out!r}"
  '';
}
