#!/bin/sh
# The Build command of a Zoomies pod on InstaPods (see docs/instapods.md):
#
#   sh deploy/instapods/fetch.sh controller   on the web pod
#   sh deploy/instapods/fetch.sh agent        on the worker pod
#
# The role can come from ZOOMIES_POD_ROLE instead of the argument, exactly as
# it can for run.sh, so one variable set on the pod keeps the two in step.
#
# It downloads a published release rather than building one. The Go preset
# carries an older Go than go.mod asks for and no Node at all, and the UI is
# built with Node before the Go build can embed it; a release binary is also
# the very file every other install runs, checked against the same
# checksums.txt the installer checks it against.
#
# The binary goes to $HOME/zoomies/bin, outside the git checkout, so the pull
# at the start of every deploy can never clobber it, and a failed build leaves
# the one that was running in place.

set -eu

REPO=eyupio/zoomies
BASE_URL="https://github.com/${REPO}/releases"
BIN_DIR="$HOME/zoomies/bin"
BIN="$BIN_DIR/zoomies"

# The checkout this script sits in: deploy/instapods/ is two levels down.
APP_DIR=$(cd "$(dirname "$0")/../.." && pwd)
# shellcheck source=deploy/instapods/lib.sh
. "$(dirname "$0")/lib.sh"

say() { printf 'zoomies: %s\n' "$*"; }
warn() { printf 'zoomies: warning: %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

fetch() {
    # fetch <url> <dest>
    if have curl; then
        curl -fsSL --retry 3 --retry-delay 2 -o "$2" "$1"
    elif have wget; then
        wget -qO "$2" "$1"
    else
        die "neither curl nor wget is on this pod, and one is needed to download Zoomies."
    fi
}

# --- release signature check: identical in connect-proxmox.sh and
# --- deploy/instapods/fetch.sh, and a test in internal/docs holds the copies equal.
# The release signing key, base64: the line `zoomies release keygen` writes to
# release.pub. A test keeps it equal to the key compiled into the binary and to
# the one in install.sh, and keeps the two functions below identical in every
# script that carries them.
RELEASE_KEY="6dK2/jDPHR8wAjsIayrxGxL8ednKVKdEMHdRcmNU67U="

# signature_required says whether a release has to carry a signature. The first
# signed release was v1.3.6.1: everything before it, and the moving dev
# prerelease, has none, so there is nothing to check and refusing them would
# refuse every version published before signing began. From v1.3.6 up a release
# that is not signed is refused, because otherwise whoever could replace the
# binary could also delete its signature, and a script that only warns would run
# what is left.
signature_required() {
    case "$1" in v[0-9]*.[0-9]*.[0-9]*) ;; *) return 1 ;; esac
    _rest=${1#v}
    _major=${_rest%%.*}
    _rest=${_rest#*.}
    _minor=${_rest%%.*}
    _rest=${_rest#*.}
    _patch=${_rest%%[!0-9]*}
    case "$_major$_minor$_patch" in '' | *[!0-9]*) return 1 ;; esac
    [ "$_major" -gt 1 ] || { [ "$_major" -eq 1 ] && { [ "$_minor" -gt 3 ] || { [ "$_minor" -eq 3 ] && [ "$_patch" -gt 5 ]; }; }; }
}

# verify_signature <tag> <checksums.txt> <checksums.txt.ed25519> <scratch dir>
# holds a release's checksums to the release key, before the checksums are
# believed. A signature that does not match always refuses. A signature that
# cannot be checked (no openssl that does Ed25519, no signature, one that cannot
# be read) refuses a release that must be signed and is a warning for one that
# cannot be, which is every release before v1.3.6.
verify_signature() {
    _tag=$1 _sums=$2 _sigfile=$3 _dir=$4
    [ -n "$RELEASE_KEY" ] || return 0
    if ! command -v openssl >/dev/null 2>&1 || ! openssl pkeyutl -help 2>&1 | grep -q rawin; then
        if signature_required "$_tag"; then
            die "$_tag is signed, and this host has no openssl (3.0 or later) to check the signature with, so the download cannot be trusted. Install openssl and run this again."
        fi
        warn "there is no openssl here that can check a signature; this download is checked by its checksum alone."
        return 0
    fi
    if [ ! -s "$_sigfile" ]; then
        if signature_required "$_tag"; then
            die "$_tag carries no checksums.txt.ed25519, and a release this new must be signed. Do not run this download."
        fi
        warn "$_tag carries no checksums.txt.ed25519, so the release key cannot check it; this download is checked by its checksum alone."
        return 0
    fi
    # The envelope is the key id, then the base64 signature.
    _sig=$(sed -n '2p' "$_sigfile")
    if ! printf '%s' "$_sig" | openssl base64 -d -A > "$_dir/sig.raw" 2>/dev/null || [ ! -s "$_dir/sig.raw" ]; then
        if signature_required "$_tag"; then
            die "the signature on $_tag could not be read. Do not run this download."
        fi
        warn "the release signature could not be read; this download is checked by its checksum alone."
        return 0
    fi
    # openssl wants the DER public key: the fixed Ed25519 SubjectPublicKeyInfo
    # prefix, then the key's 32 bytes.
    printf '\060\052\060\005\006\003\053\145\160\003\041\000' > "$_dir/key.prefix"
    if ! printf '%s' "$RELEASE_KEY" | openssl base64 -d -A > "$_dir/key.raw" 2>/dev/null || [ ! -s "$_dir/key.raw" ]; then
        die "the release key built into this script cannot be read; copy the script again from https://zoomies.sh."
    fi
    cat "$_dir/key.prefix" "$_dir/key.raw" > "$_dir/release.der"
    if openssl pkeyutl -verify -pubin -keyform DER -inkey "$_dir/release.der" \
        -rawin -in "$_sums" -sigfile "$_dir/sig.raw" >/dev/null 2>&1; then
        say "the release signature matches the release key"
    else
        die "the release signature on $_tag does not match the release key. Do not run this download."
    fi
}
# --- end of the release signature check.

sha256_of() {
    if have sha256sum; then
        sha256sum "$1" | cut -d' ' -f1
    elif have shasum; then
        shasum -a 256 "$1" | cut -d' ' -f1
    else
        printf ''
    fi
}

# resolve_latest names the tag /releases/latest redirects to. While every
# release is a prerelease GitHub redirects to the release index instead, so
# the newest v-tag from the API is the fallback; the moving dev prerelease is
# never what "latest" means.
resolve_latest() {
    url=''
    if have curl; then
        url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/latest" 2>/dev/null || printf '')
    fi
    case "$url" in
        */tag/*)
            printf '%s\n' "${url##*/tag/}"
            return 0
            ;;
    esac
    api="https://api.github.com/repos/${REPO}/releases?per_page=20"
    body=$(curl -fsSL "$api" 2>/dev/null || printf '')
    printf '%s\n' "$body" | grep -o '"tag_name"[^,]*' |
        sed 's/.*: *"//; s/".*//' | grep '^v' | head -1
}

# as_root runs a command as root: directly when this already is root, through
# sudo when sudo needs no password, and not at all otherwise. A Build command
# has no terminal, so a sudo that asks for a password would hang the deploy
# instead of failing it.
as_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    else
        sudo -n "$@"
    fi
}

# runner_libraries makes sure the native libraries actions/runner loads are
# installed. The process backend runs the runner straight on this pod, with no
# image to bring them, and a runner missing libicu exits at startup with an
# error that names .NET rather than the package. The names are Ubuntu 24.04's,
# which is what a pod runs.
runner_libraries() {
    set -- libicu74 libkrb5-3 zlib1g libssl3t64 liblttng-ust1t64
    if ! have dpkg-query; then
        warn "this is not a Debian or Ubuntu system, so the runner's libraries were not checked." \
            "The process backend needs ICU, Kerberos, zlib, OpenSSL and LTTng-UST installed."
        return 0
    fi
    # shellcheck disable=SC1091 # the system's file, read for two fields
    release=$(. /etc/os-release 2>/dev/null && printf '%s %s' "${ID:-}" "${VERSION_ID:-}" || printf '')
    if [ "$release" != "ubuntu 24.04" ]; then
        warn "the runner's library names here are Ubuntu 24.04's and this is ${release:-an unknown release}." \
            "If a runner exits at startup, install what actions/runner's bin/installdependencies.sh lists."
        return 0
    fi
    missing=''
    for pkg in "$@"; do
        # shellcheck disable=SC2016 # ${Status} is dpkg-query's format, not ours
        status=$(dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null || printf '')
        case "$status" in
            *' installed') ;;
            *) missing="$missing $pkg" ;;
        esac
    done
    missing=${missing# }
    if [ -z "$missing" ]; then
        say "the runner's libraries are installed"
        return 0
    fi
    if [ "$(id -u)" -ne 0 ] && ! sudo -n true 2>/dev/null; then
        die "the GitHub Actions runner needs packages this pod does not have: $missing" \
            "This user cannot run sudo without a password, so install them from the pod's" \
            "Web Terminal or over SSH, then deploy again:" \
            "  sudo apt-get update && sudo apt-get install -y $missing"
    fi
    say "installing the runner's libraries: $missing"
    # Word splitting is the point: $missing is a list of package names.
    # shellcheck disable=SC2086
    if ! { as_root env DEBIAN_FRONTEND=noninteractive apt-get update -qq &&
        as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends $missing; }; then
        die "installing $missing failed (the apt-get output is above)." \
            "Install them from the pod's Web Terminal, then deploy again:" \
            "  sudo apt-get update && sudo apt-get install -y $missing"
    fi
}

# write_launcher puts a small script at the path the Go preset's service runs
# when no Start command is in force. The preset documents that its service
# runs $APP_DIR/app; the Start command this guide sets is meant to replace
# that, and this keeps a pod whose Start command was cleared or never saved
# running the right role instead of a missing file.
write_launcher() {
    launcher="$APP_DIR/app"
    if [ -e "$launcher" ] && [ ! -f "$launcher" ]; then
        warn "$launcher exists and is not a file, so no launcher was written there."
        return 0
    fi
    tmp_launcher="$launcher.zoomies.$$"
    cat >"$tmp_launcher" <<EOF
#!/bin/sh
# Written by deploy/instapods/fetch.sh on every deploy; edits here are lost.
exec sh "$APP_DIR/deploy/instapods/run.sh" $ROLE "\$@"
EOF
    chmod 0755 "$tmp_launcher"
    mv -f "$tmp_launcher" "$launcher"
}

load_env "$APP_DIR/.env"

ROLE=${1:-${ZOOMIES_POD_ROLE:-}}
case "$ROLE" in
    controller | agent) ;;
    '') die "say which pod this is: sh deploy/instapods/fetch.sh controller (the web pod)" \
        "or sh deploy/instapods/fetch.sh agent (the worker pod), or set ZOOMIES_POD_ROLE." ;;
    *) die "$ROLE is not a role: use controller (the web pod) or agent (the worker pod)." ;;
esac

[ "$(uname -s)" = Linux ] || die "this script is for an InstaPods pod, which runs Linux."
case "$(uname -m)" in
    x86_64 | amd64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) die "$(uname -m) is not an architecture Zoomies ships: there are amd64 and arm64 builds." ;;
esac
ASSET="zoomies_linux_${ARCH}"

VERSION=${ZOOMIES_VERSION:-latest}
if [ "$VERSION" = latest ]; then
    TAG=$(resolve_latest)
    [ -n "$TAG" ] || die "could not work out which release is the latest." \
        "Set ZOOMIES_VERSION to a tag (dev, or v1.2.3) with instapods env set, and deploy again."
else
    TAG=$VERSION
    # Numeric versions keep the project's v-prefixed tag spelling.
    case "$TAG" in dev | v*) ;; *) TAG="v$TAG" ;; esac
fi
say "release $TAG ($VERSION), $ASSET"

mkdir -p "$BIN_DIR"
WORK=$(mktemp -d "$BIN_DIR/.fetch.XXXXXX")
# shellcheck disable=SC2064 # expand now: the directory is fixed from here on
trap "rm -rf '$WORK'" EXIT
trap 'exit 1' INT TERM

# A download that cannot be verified is refused, never installed with a
# warning: a deploy log is not somewhere anybody reads a warning.
fetch "$BASE_URL/download/$TAG/checksums.txt" "$WORK/checksums.txt" ||
    die "could not download checksums.txt for $TAG." \
        "$BASE_URL/download/$TAG/checksums.txt" \
        "Check that $TAG exists at $BASE_URL. If it was published minutes ago its" \
        "binaries may still be uploading; deploy again shortly."
# A release before v1.3.6 has no signature; the file is simply absent then.
fetch "$BASE_URL/download/$TAG/checksums.txt.ed25519" "$WORK/checksums.txt.ed25519" 2>/dev/null ||
    rm -f "$WORK/checksums.txt.ed25519"
# Before the checksums are believed: a checksum from the same place as the binary
# says only that the download was not corrupted on the way.
verify_signature "$TAG" "$WORK/checksums.txt" "$WORK/checksums.txt.ed25519" "$WORK"
WANT=$(awk -v a="$ASSET" '$2 == a || $2 == "*"a {print $1; exit}' "$WORK/checksums.txt")
[ -n "$WANT" ] || die "checksums.txt for $TAG has no entry for $ASSET, so a download could not be verified." \
    "Choose another release with ZOOMIES_VERSION, and deploy again."

if [ -x "$BIN" ] && [ "$(sha256_of "$BIN")" = "$WANT" ]; then
    # Nothing to fetch: the same deploy run twice, or an env change that
    # did not move the version, should not cost a 40 MB download.
    say "already have $ASSET for $TAG"
else
    fetch "$BASE_URL/download/$TAG/$ASSET" "$WORK/zoomies" ||
        die "could not download $ASSET for $TAG from $BASE_URL/download/$TAG/$ASSET." \
            "Check that the release exists and has finished uploading, then deploy again."
    GOT=$(sha256_of "$WORK/zoomies")
    [ -n "$GOT" ] || die "there is no sha256sum or shasum on this pod to check the download with."
    if [ "$GOT" != "$WANT" ]; then
        die "checksum mismatch for $ASSET $TAG: the download is not what the release published." \
            "expected $WANT" \
            "got      $GOT" \
            "It was not installed, and the binary already here was left alone. Deploy again," \
            "and if it happens twice, report it at https://github.com/${REPO}/issues."
    fi
    chmod 0755 "$WORK/zoomies"
    # Running it once catches a binary that cannot execute here before it
    # replaces one that can.
    "$WORK/zoomies" version >/dev/null 2>&1 ||
        die "the downloaded $ASSET does not run on this pod; the binary already here was left alone."
    # A rename within one directory is atomic, and a running process keeps
    # the file it started from, so the service is never left without one.
    mv -f "$WORK/zoomies" "$BIN"
    say "sha256 $(printf '%.12s' "$GOT")... matches; installed $BIN"
fi

if [ "$ROLE" = agent ]; then
    runner_libraries
fi

write_launcher
"$BIN" version | head -1 | sed 's/^/zoomies: ready: /'
