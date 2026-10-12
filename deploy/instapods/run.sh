#!/bin/sh
# The Start command of a Zoomies pod on InstaPods (see docs/instapods.md):
#
#   sh deploy/instapods/run.sh controller   on the web pod
#   sh deploy/instapods/run.sh agent        on the worker pod
#
# The role can come from ZOOMIES_POD_ROLE instead of the argument, as it can
# for fetch.sh. Everything here is a default: a ZOOMIES_* variable set with
# `instapods env set` wins over it, and the binary is the one fetch.sh put in
# $HOME/zoomies/bin during the Build step.

set -eu

APP_DIR=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck source=deploy/instapods/lib.sh
. "$(dirname "$0")/lib.sh"

load_env "$APP_DIR/.env"

ROLE=${1:-${ZOOMIES_POD_ROLE:-}}
case "$ROLE" in
    controller | agent) ;;
    '') die "say which pod this is: sh deploy/instapods/run.sh controller (the web pod)" \
        "or sh deploy/instapods/run.sh agent (the worker pod), or set ZOOMIES_POD_ROLE." ;;
    *) die "$ROLE is not a role: use controller (the web pod) or agent (the worker pod)." ;;
esac

# ZOOMIES_BIN points this at another binary, for trying a build of your own
# on the pod without publishing it.
ZOOMIES_BIN=${ZOOMIES_BIN:-$HOME/zoomies/bin/zoomies}
[ -x "$ZOOMIES_BIN" ] || die "there is no Zoomies binary at $ZOOMIES_BIN." \
    "The Build command (sh deploy/instapods/fetch.sh $ROLE) downloads it there;" \
    "read the last deploy's build log for why it did not, and deploy again."

# Both pods keep everything under $HOME/zoomies, beside the binary and outside
# the git checkout, so a redeploy's pull never touches the database, the
# encryption key or the agent's credentials. Pointing ZOOMIES_CONFIG_DIR there
# too means the default zoomies.yaml path is one this user owns: the file need
# not exist, and environment variables alone configure the process.
: "${ZOOMIES_STATE_DIR:=$HOME/zoomies/data}"
: "${ZOOMIES_CONFIG_DIR:=$HOME/zoomies/data}"
: "${ZOOMIES_LOG_FORMAT:=text}"
export ZOOMIES_STATE_DIR ZOOMIES_CONFIG_DIR ZOOMIES_LOG_FORMAT
# The controller opens its database lock without creating the folder first,
# which an installer or a mounted volume normally has. Private to this user:
# the folder holds the database and, unless one is supplied, the key.
(umask 077 && mkdir -p "$ZOOMIES_STATE_DIR" "$ZOOMIES_CONFIG_DIR")

case "$ROLE" in
    controller)
        # InstaPods proxies the pod's https address to the port in $PORT
        # (8080) and holds the certificate, so the controller listens in the
        # clear on every interface and says so at startup; behind this proxy
        # that warning is expected.
        : "${ZOOMIES_BIND:=0.0.0.0:${PORT:-8080}}"
        : "${ZOOMIES_TLS_MODE:=off}"
        # A pod has no Docker, so an agent inside the controller could never
        # start a runner. Runners come from the worker pod, which also keeps
        # every job away from the database and the GitHub App's key.
        : "${ZOOMIES_AGENT_EMBEDDED:=false}"
        export ZOOMIES_BIND ZOOMIES_TLS_MODE ZOOMIES_AGENT_EMBEDDED
        exec "$ZOOMIES_BIN" controller
        ;;
    agent)
        # No Docker on a pod, so each runner is a process in it: see the
        # process backend in docs/security.md for what that means.
        : "${ZOOMIES_AGENT_BACKEND:=process}"
        : "${ZOOMIES_WORK_DIR:=$HOME/zoomies/work}"
        export ZOOMIES_AGENT_BACKEND ZOOMIES_WORK_DIR
        [ -n "${ZOOMIES_CONTROLLER_URL:-}" ] ||
            die "ZOOMIES_CONTROLLER_URL is not set. Set it to the web pod's https address:" \
                "  instapods env set <worker-pod> ZOOMIES_CONTROLLER_URL=https://<web-pod>.<region>.instapods.app" \
                "It stays set: the agent reads it on every start, not only the first."
        # The agent keeps the credentials it was given when it joined in
        # agent.json in its work directory. Only a pod without them needs a
        # join token, and a token is single use, so asking for one here would
        # be wrong on every start after the first.
        if [ ! -s "$ZOOMIES_WORK_DIR/agent.json" ] && [ -z "${ZOOMIES_JOIN_TOKEN:-}" ]; then
            die "this pod has not joined the controller yet, and ZOOMIES_JOIN_TOKEN is not set." \
                "Mint a join token on the controller (Hosts, then Add a host, or" \
                "zoomies hosts join-token create --ttl 15m) and give it to this pod:" \
                "  instapods env set <worker-pod> ZOOMIES_JOIN_TOKEN=zoojoin_..." \
                "It is needed once: the credentials it buys are kept in $ZOOMIES_WORK_DIR/agent.json."
        fi
        exec "$ZOOMIES_BIN" agent
        ;;
esac
