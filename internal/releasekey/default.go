package releasekey

// Default is the release key set compiled into the binary: one base64 public
// key per line, so more than one is carried while a key is rotated. It is what
// `zoomies upgrade` and `zoomies release verify` hold a release's signature to.
//
// It is empty until the owner mints a key and commits the public half (docs/
// upgrading.md, "Signing releases"). An empty set means a download is not held
// to a signature, which the download says once. Set it to the line
// `zoomies release keygen --out DIR` wrote to release.pub; keep more than one
// line only for the releases of a rotation.
// It is a variable rather than a constant so a test can set it and put it
// back; nothing else writes it.
var Default = ""
