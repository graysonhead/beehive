{ pkgs, beehive, honeybee }:

pkgs.testers.nixosTest {
  name = "beehive-honeybee-schedule";

  nodes.machine = { pkgs, ... }: {
    imports = [ ../../module.nix ];

    _module.args.inputs = { };

    services.beehived = {
      enable = false;
      repoRoot = "/var/lib/beehived-test/repo";
      honeybee = {
        enable = true;
        package = honeybee;
        # exercised directly by the test script below rather than waiting on
        # the real calendar schedule
        schedule = "*-*-* *:*:00";
      };
    };
  };

  testScript = ''
    machine.start()

    # No interactive session: honeybee.service should be allowed to start
    # (it will fail past ExecStartPre for lack of a real repo, but that's a
    # separate concern — what we're checking here is the idle gate itself).
    machine.systemctl("start honeybee.service")
    machine.wait_until_fails("systemctl is-active --quiet honeybee.service")
    machine.succeed(
        "journalctl -u honeybee.service | grep -qv 'skipping run, interactive'"
    )

    # Simulate an interactive login the way a real console/tty login does —
    # through the "login" PAM service so logind registers a genuine
    # Class=user session, not just a lingering user-manager (Class=manager,
    # which the gate must NOT treat as an interactive user).
    machine.execute(
        "systemd-run --unit=fake-login -p PAMName=login "
        "-p TTYPath=/dev/tty7 -p Type=exec sleep 300"
    )
    machine.succeed(
        "for i in $(seq 20); do "
        "loginctl list-sessions --no-legend | grep -q . && break; sleep 0.5; "
        "done"
    )
    machine.succeed("loginctl list-sessions --no-legend | grep -q .")
    session_id = machine.succeed(
        "loginctl list-sessions --no-legend | awk '{print $1}'"
    ).split()[0]
    session_class = machine.succeed(
        f"loginctl show-session {session_id} -p Class --value"
    ).strip()
    assert session_class in ("user", "user-early"), (
        f"expected an interactive-user logind session, got class={session_class!r}"
    )
    machine.systemctl("start honeybee.service")
    machine.wait_until_fails("systemctl is-active --quiet honeybee.service")
    machine.succeed(
        "journalctl -u honeybee.service -n 20 | grep -q 'skipping run, interactive'"
    )

    machine.succeed("systemctl list-timers honeybee.timer | grep -q honeybee.timer")
  '';
}
