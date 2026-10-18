#!/bin/sh
# Brings a Zoomies pod on InstaPods up to date (see docs/instapods.md):
#
#   sh deploy/instapods/update.sh             this pod, now
#   sh deploy/instapods/update.sh --force     the worker, even with a job running
#   sh deploy/instapods/update.sh controller  name the role when it cannot be told
#
# It does what the dashboard's Deploy does, for a pod that has no Deploy button:
# it moves the checkout to the newest commit, runs the Build command
# (fetch.sh, which downloads the release the pod's ZOOMIES_VERSION names and
# checks it against the release's checksums) and restarts the app. The controller
# pod has a Deploy button and can use that instead; this works on both, so one
# command keeps the pair in step.
#
# Nothing is restarted when nothing changed, so it is safe to run on a schedule.
#
# The checkout is reset to the remote's commit, so a hand edit to a tracked file
# in ~/app is discarded. The environment file, the database, the encryption key
# and the agent's credentials are not in the checkout and are never touched.

set -eu

APP_DIR=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck source=deploy/instapods/lib.sh
. "$(dirname "$0")/lib.sh"

say() { printf 'zoomies: %s\n' "$*"; }
warn() { printf 'zoomies: warning: %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

# sha_of is empty for a file that is not there, which is what "nothing
# installed yet" should compare as.
sha_of() {
    if [ -f "$1" ]; then
        sha256sum "$1" | cut -d' ' -f1
    else
        printf ''
    fi
}

# job_running says whether the Actions runner has a job in progress. A job runs
# in a Runner.Worker process the runner starts for it, and restarting the agent
# kills that process and fails the job. The bracket keeps this very command line
# from matching its own pattern.
job_running() {
    grep -qa 'Runner[.]Worker' /proc/[0-9]*/cmdline 2>/dev/null
}

# job_blocks_restart <role> <force> says whether a restart now would fail a job:
# this is the worker, --force was not given, and a job is in progress.
job_blocks_restart() {
    [ "$1" = agent ] && [ "$2" -eq 0 ] && job_running
}

# restart_app restarts the pod's app service, and returns 1 when there is no
# service here to restart, so that the caller does not claim the new version is
# running. reset-failed first, because a service that crashed on its way down (a
# missing token, say) can be refused a restart for starting too often.
restart_app() {
    if ! have systemctl || ! systemctl cat app.service >/dev/null 2>&1; then
        say "restart the app to run the new version: sudo systemctl restart app"
        return 1
    fi
    if [ "$(id -u)" -eq 0 ]; then
        systemctl reset-failed app 2>/dev/null || true
        systemctl restart app
    elif sudo -n true 2>/dev/null; then
        sudo -n systemctl reset-failed app 2>/dev/null || true
        sudo -n systemctl restart app
    else
        die "the new version is installed but the app was not restarted:" \
            "this user cannot run sudo without a password. Restart it from the" \
            "pod's Terminal or its dashboard:" \
            "  sudo systemctl restart app"
    fi
    sleep 3
    if ! systemctl is-active --quiet app; then
        die "the app did not stay up after the restart. Read why with:" \
            "  journalctl -u app -n 30 --no-pager"
    fi
    say "restarted the app"
}

# The whole script is one function, called on the same line that ends the file.
# A shell reads a script as it runs it, and the checkout reset below can replace
# this very file; a function is read in full before it runs, so the new contents
# are never half-read in the middle of the old ones.
main() {
    force=0
    role=''
    for arg in "$@"; do
        case "$arg" in
            --force) force=1 ;;
            controller | agent) role=$arg ;;
            *) die "$arg is not something this takes." \
                "usage: sh deploy/instapods/update.sh [--force] [controller|agent]" ;;
        esac
    done

    load_env "$APP_DIR/.env"

    # A pod that points at a controller is the worker; the web pod points at
    # nothing. ZOOMIES_POD_ROLE says so outright when it is set.
    if [ -z "$role" ]; then role=${ZOOMIES_POD_ROLE:-}; fi
    if [ -z "$role" ]; then
        if [ -n "${ZOOMIES_CONTROLLER_URL:-}" ]; then
            role=agent
        else
            role=controller
        fi
    fi

    if job_blocks_restart "$role" "$force"; then
        say "a job is running here, and restarting would fail it, so nothing was changed."
        say "run this again when it has finished, or with --force to update anyway."
        return 0
    fi

    bin=${ZOOMIES_BIN:-$HOME/zoomies/bin/zoomies}
    # Written when a job holds a restart back, and read by the next run: with the
    # new binary already in place that run would otherwise find nothing to do,
    # and the app would go on running the old one.
    pending="$(dirname "$bin")/restart-pending"
    before_bin=$(sha_of "$bin")
    before_scripts=''

    if [ -d "$APP_DIR/.git" ]; then
        before_scripts=$(git -C "$APP_DIR" rev-parse HEAD:deploy/instapods 2>/dev/null || printf '')
        branch=$(git -C "$APP_DIR" symbolic-ref --short -q HEAD || printf 'main')
        say "moving the checkout to the newest $branch"
        git -C "$APP_DIR" fetch -q --depth 1 origin "$branch"
        git -C "$APP_DIR" reset -q --hard FETCH_HEAD
    else
        warn "$APP_DIR is not a git checkout, so its scripts were not updated."
    fi

    sh "$APP_DIR/deploy/instapods/fetch.sh" "$role"

    after_bin=$(sha_of "$bin")
    after_scripts=''
    if [ -d "$APP_DIR/.git" ]; then
        after_scripts=$(git -C "$APP_DIR" rev-parse HEAD:deploy/instapods 2>/dev/null || printf '')
    fi

    # The scripts are compared as well as the binary: they decide how the app
    # starts, but most commits to the repository change neither, and a pod that
    # restarted for every one of them would restart daily for nothing.
    if [ "$before_bin" = "$after_bin" ] && [ "$before_scripts" = "$after_scripts" ] && [ ! -e "$pending" ]; then
        say "already up to date; nothing was restarted."
        return 0
    fi

    # The check at the top was made before the download, and a download takes
    # minutes: a job that started in them would be failed by this restart.
    if job_blocks_restart "$role" "$force"; then
        mkdir -p "$(dirname "$pending")"
        : >"$pending"
        say "a job started while this update was running, so the app was not restarted and the job was left to finish."
        say "the new version is installed. Run this again when the job has finished, or with --force to restart now."
        return 0
    fi

    if restart_app; then
        state="now running"
    else
        state="installed"
    fi
    rm -f "$pending"
    "$bin" version | head -1 | sed "s/^/zoomies: $state: /"
}

main "$@"; exit $?
