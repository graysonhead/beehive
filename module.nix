{ config, lib, pkgs, inputs, ... }:

with lib;

let
  cfg = config.services.beehived;
in
{
  options = {
    services.beehived = {
      enable = mkEnableOption "beehived";

      package = mkOption {
        default = inputs.beehive.packages.${pkgs.system}.beehived;
        defaultText = literalExpression "inputs.beehive.packages.<system>.beehived";
        type = types.package;
      };

      repoRoot = mkOption {
        type = types.path;
        description = ''
          Path to the beehive repo root that beehived serves. beehived resolves
          its own configuration (repos.yaml / config.yaml) from within this
          repo, so this module intentionally does not manage that config —
          it only wires up the service.
        '';
      };

      listenAddress = mkOption {
        type = types.str;
        default = ":8955";
        description = "Address beehived listens on (passed as -addr).";
      };

      honeybee = {
        enable = mkEnableOption "a scheduled honeybee task runner";

        package = mkOption {
          default = inputs.beehive.packages.${pkgs.system}.honeybee;
          defaultText = literalExpression "inputs.beehive.packages.<system>.honeybee";
          type = types.package;
        };

        repoRoot = mkOption {
          type = types.path;
          default = cfg.repoRoot;
          defaultText = literalExpression "config.services.beehived.repoRoot";
          description = "Repo root passed to honeybee as its working root.";
        };

        schedule = mkOption {
          type = types.str;
          default = "*:0/15";
          description = ''
            systemd.time(7) OnCalendar expression for how often to run
            honeybee (default: every 15 minutes).
          '';
        };

        requireNoLoggedInUsers = mkOption {
          type = types.bool;
          default = true;
          description = ''
            Only start a honeybee run when no user has an interactive
            (non-greeter) logind session — a honeybee session drives an LLM
            agent that edits the repo and can be resource-heavy, so this
            avoids contending with someone actively using the machine. Each
            scheduled tick that finds a user logged in is skipped, not
            queued; the next tick tries again.
          '';
        };
      };
    };
  };

  config = mkMerge [
    (mkIf cfg.enable {
      systemd.services.beehived = {
        description = "beehived";
        after = [ "network.target" ];
        wantedBy = [ "multi-user.target" ];
        # beehived and its startup housekeeping shell out to `git` for every repo
        # op, and secrets features shell out to `gpg` — without these on the
        # unit's own PATH it depends on ambient PATH, which a minimal systemd
        # unit doesn't have. `bash` is needed too: `beehive init`-installed git
        # hooks have a `#!/usr/bin/env sh` shebang, and git invokes them as part
        # of any commit beehived makes. `openssh` provides `ssh`, which git
        # shells out to for ssh:// / git@ remotes.
        path = [ pkgs.git pkgs.gnupg pkgs.bash pkgs.openssh ];
        serviceConfig = {
          Type = "simple";
          ExecStart = "${cfg.package}/bin/beehived -repo ${cfg.repoRoot} -addr ${cfg.listenAddress}";
          Restart = "on-failure";
          RestartSec = 2;
        };
      };
    })

    (mkIf cfg.honeybee.enable {
      systemd.services.honeybee = {
        description = "honeybee task runner (one selection/claim/run cycle)";
        after = [ "network.target" ];
        path = [ pkgs.git pkgs.gnupg pkgs.bash pkgs.openssh ];
        serviceConfig = {
          Type = "oneshot";
          ExecStartPre = mkIf cfg.honeybee.requireNoLoggedInUsers
            "${pkgs.writeShellScript "honeybee-check-idle" ''
              set -eu
              # A logind session's Class (queried per-session; list-sessions
              # itself has no such column) is "user"/"user-early"/
              # "user-incomplete" for a real interactive login (the
              # "-early"/"-incomplete" suffixes are live states a session
              # can sit in, not just transient log-line wording — a session
              # can report Class=user-early well after login), vs
              # "greeter"/"lock-screen"/"background"/"manager"* for the
              # rest. Skip this run (exit 1 fails ExecStartPre, which
              # aborts the oneshot without touching ExecStart) rather than
              # queuing — the next timer tick will just try again.
              for s in $(${pkgs.systemd}/bin/loginctl list-sessions --no-legend | ${pkgs.gawk}/bin/awk '{print $1}'); do
                class="$(${pkgs.systemd}/bin/loginctl show-session "$s" -p Class --value)"
                case "$class" in
                  user*)
                    echo "honeybee: skipping run, interactive user(s) logged in" >&2
                    exit 1
                    ;;
                esac
              done
            ''}";
          ExecStart = "${cfg.honeybee.package}/bin/honeybee ${cfg.honeybee.repoRoot}";
        };
      };

      systemd.timers.honeybee = {
        description = "Run honeybee on a schedule";
        wantedBy = [ "timers.target" ];
        timerConfig = {
          OnCalendar = cfg.honeybee.schedule;
          Unit = "honeybee.service";
          Persistent = false;
        };
      };
    })
  ];
}
