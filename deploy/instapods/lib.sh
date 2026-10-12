# Shared by fetch.sh and run.sh, the Build and Start commands of a Zoomies pod
# on InstaPods (see docs/instapods.md). Sourced, never run.
# shellcheck shell=sh

die() {
    printf 'zoomies: %s\n' "$1" >&2
    shift
    for line in "$@"; do
        printf '  %s\n' "$line" >&2
    done
    exit 1
}

# load_env reads the platform's env file without overriding anything already
# in the environment. Both commands need it: a deploy's Build step is not
# documented to see the variables `instapods env set` stores, and
# ZOOMIES_VERSION is one of them; and the Start step's service may be handed
# only some of them (none after a hand edit of the file, until the next `env
# set`), while a controller that came up without ZOOMIES_ENCRYPTION_KEY would
# generate a key of its own rather than fail. Simple KEY=value lines only: a
# quoted value spanning several lines is skipped rather than half-read.
load_env() {
    [ -r "$1" ] || return 0
    cr=$(printf '\r')
    quote=''
    while IFS= read -r line || [ -n "$line" ]; do
        line=${line%"$cr"}
        if [ -n "$quote" ]; then
            case "$line" in *"$quote") quote='' ;; esac
            continue
        fi
        line=${line#"${line%%[![:space:]]*}"}
        case "$line" in '' | '#'*) continue ;; esac
        line=${line#export }
        key=${line%%=*}
        [ "$key" != "$line" ] || continue
        case "$key" in '' | [0-9]* | *[!A-Za-z0-9_]*) continue ;; esac
        value=${line#*=}
        case "$value" in
            \"*\" | \'*\')
                value=${value#?}
                value=${value%?}
                ;;
            \"* | \'*)
                quote=${value%"${value#?}"}
                continue
                ;;
            *)
                value=${value%%[[:space:]]#*}
                value=${value%"${value##*[![:space:]]}"}
                ;;
        esac
        printenv "$key" >/dev/null 2>&1 && continue
        export "$key=$value"
    done <"$1"
}
