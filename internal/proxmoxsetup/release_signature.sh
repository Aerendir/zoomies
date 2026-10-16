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
