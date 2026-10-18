#!/bin/sh
# Zoomies Proxmox connector, the short form of the setup command the provider
# wizard generates.
#
#   curl -fsSL https://zoomies.sh/connect-proxmox.sh | sudo sh -s -- \
#       --version <tag> --key <key> --controller <url> --setup-id <id> --token <token>
#
# The wizard prints the whole line with the values filled in. Reading this file
# first is safe, and recommended: it downloads one release binary, checks the
# release's checksums.txt against the release signing key and the binary's SHA-256
# against those checksums, keeps a private copy under /var/lib/zoomies-proxmox and
# runs `zoomies providers connect-proxmox` with the values it was given. It never
# touches an installed runner host: no /usr/local/bin, no zoomies-agent unit, no
# join token.
#
# This is the same work the wizard's "Show the full command" text does, in a
# file that is checked in CI instead of pasted from a browser.
#
# POSIX sh, checked with dash in CI.

set -eu
umask 077

die() { printf 'zoomies: %s\n' "$*" >&2; exit 1; }
say() { printf 'zoomies: %s\n' "$*" >&2; }
warn() { printf 'zoomies: warning: %s\n' "$*" >&2; }

# --- release signature check: identical in connect-proxmox.sh and
# --- deploy/instapods/fetch.sh, and a test in internal/docs holds the copies equal.
# The release signing key, base64: the line `zoomies release keygen` writes to
# release.pub. A test keeps it equal to the key compiled into the binary and to
# the one in install.sh, and keeps the two functions below identical in every
# script that carries them.
RELEASE_KEY="6dK2/jDPHR8wAjsIayrxGxL8ednKVKdEMHdRcmNU67U="

# signature_required says whether a release has to carry a signature. The first
# signed release was v1.3.6.1: everything before it has none, so there is
# nothing to check and refusing them would refuse every version published before
# signing began, and the moving dev prerelease is never signed. From v1.3.6 up a
# release that is not signed is refused, because otherwise whoever could replace
# the binary could also delete its signature, and a script that only warns would
# run what is left.
#
# A name this cannot read as three short numbers is held to a signature as well.
# Whoever publishes a release picks its name, so a name nobody can place must
# never be the way out of the check. A number is compared only when it is short
# enough to compare: the shell's [ answers one that overflows with an error
# status, which a caller would read as "not required".
signature_required() {
    [ "$1" != dev ] || return 1
    case "$1" in v[0-9]*.[0-9]*.[0-9]*) ;; *) return 0 ;; esac
    _rest=${1#v}
    _major=${_rest%%.*}
    _rest=${_rest#*.}
    _minor=${_rest%%.*}
    _rest=${_rest#*.}
    _patch=${_rest%%[!0-9]*}
    for _n in "$_major" "$_minor" "$_patch"; do
        case $_n in [0-9] | [0-9][0-9] | [0-9][0-9][0-9] | [0-9][0-9][0-9][0-9]) ;; *) return 0 ;; esac
    done
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
    # prefix, then the key's 32 bytes. While a key is rotated RELEASE_KEY holds
    # more than one, one per line, exactly as the binary's key set does, and the
    # signature has to verify under one of them.
    printf '\060\052\060\005\006\003\053\145\160\003\041\000' > "$_dir/key.prefix"
    printf '%s\n' "$RELEASE_KEY" > "$_dir/keys.txt"
    _keys=0 _matched=0
    while IFS= read -r _key; do
        _key=$(printf '%s' "$_key" | tr -d '[:space:]')
        [ -n "$_key" ] || continue
        _keys=$((_keys + 1))
        if ! printf '%s' "$_key" | openssl base64 -d -A > "$_dir/key.raw" 2>/dev/null || [ ! -s "$_dir/key.raw" ]; then
            die "the release key built into this script cannot be read; copy the script again from https://zoomies.sh."
        fi
        cat "$_dir/key.prefix" "$_dir/key.raw" > "$_dir/release.der"
        if openssl pkeyutl -verify -pubin -keyform DER -inkey "$_dir/release.der" \
            -rawin -in "$_sums" -sigfile "$_dir/sig.raw" >/dev/null 2>&1; then
            _matched=1
            break
        fi
    done < "$_dir/keys.txt"
    [ "$_keys" -gt 0 ] || die "the release key built into this script cannot be read; copy the script again from https://zoomies.sh."
    if [ "$_matched" -eq 1 ]; then
        say "the release signature matches the release key"
    else
        die "the release signature on $_tag does not match the release key. Do not run this download."
    fi
}
# --- end of the release signature check.

# Everything runs inside main, called on the last line, so a transfer cut short
# is a syntax error rather than half a setup.
main() {
    version="" key="" controller="" setup_id="" token=""
    while [ $# -gt 0 ]; do
        [ $# -ge 2 ] || die "$1 needs a value; copy the complete command from the provider wizard."
        case "$1" in
            --version)    version=$2 ;;
            --key)        key=$2 ;;
            --controller) controller=$2 ;;
            --setup-id)   setup_id=$2 ;;
            --token)      token=$2 ;;
            *)            die "unknown option $1; copy the complete command from the provider wizard." ;;
        esac
        shift 2
    done
    if [ -z "$version" ] || [ -z "$key" ] || [ -z "$controller" ] || [ -z "$setup_id" ] || [ -z "$token" ]; then
        die "a value is missing; copy the complete command from the provider wizard."
    fi

    # The values name a directory and a download, so they are held to the
    # characters a tag and a key are made of before they are used for either.
    case "$version$key" in
        *[!A-Za-z0-9._-]*) die "the version or key contains a character that is not allowed; copy the complete command from the provider wizard." ;;
    esac

    [ "$(id -u)" -eq 0 ] || die "run this as root, for example: curl -fsSL https://zoomies.sh/connect-proxmox.sh | sudo sh -s -- ..."
    [ "$(uname -s)" = Linux ] || die "run this on the Proxmox host, which is Linux."

    dir="/var/lib/zoomies-proxmox/$key"
    mkdir -p "$dir"

    # A setup that already completed on this host reuses its binary, which may
    # have been upgraded since, instead of downloading another.
    receipt=$(printf '%s%s' "$setup_id" "$token" | sha256sum | cut -c1-12)
    if [ -f "$dir/completed-$receipt" ] && [ -x "$dir/zoomies" ]; then
        exec "$dir/zoomies" providers connect-proxmox --controller "$controller" --setup-id "$setup_id" --token "$token"
    fi

    case $(uname -m) in
        x86_64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) die "unsupported host architecture $(uname -m); Zoomies ships amd64 and arm64." ;;
    esac
    asset="zoomies_linux_$arch"
    base="https://github.com/eyupio/zoomies/releases/download/$version"

    tmp=$(mktemp -d "$dir/download.XXXXXX")
    trap 'rm -rf "$tmp"' EXIT
    curl --proto '=https' --proto-redir '=https' -fsSL "$base/$asset" -o "$tmp/$asset"
    curl --proto '=https' --proto-redir '=https' -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
    # A release before v1.3.6 has no signature; the file is simply absent then.
    curl --proto '=https' --proto-redir '=https' -fsSL "$base/checksums.txt.ed25519" -o "$tmp/checksums.txt.ed25519" 2>/dev/null ||
        rm -f "$tmp/checksums.txt.ed25519"
    # Before the checksums are believed: a checksum from the same place as the
    # binary says only that the download was not corrupted on the way.
    verify_signature "$version" "$tmp/checksums.txt" "$tmp/checksums.txt.ed25519" "$tmp"
    (
        cd "$tmp"
        awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print; found=1 } END { if (!found) exit 1 }' checksums.txt > selected ||
            die "checksums.txt has no entry for $asset."
        sha256sum -c selected
    )
    chmod 700 "$tmp/$asset"
    # Not exec: the trap has to remove the download once the connector is done.
    "$tmp/$asset" providers connect-proxmox --controller "$controller" --setup-id "$setup_id" --token "$token"
}

main "$@"
