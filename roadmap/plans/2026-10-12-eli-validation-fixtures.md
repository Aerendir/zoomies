# Eli enhancement validation: explicit storage fixture permissions

Follow-up to merged #959 while finishing the full repository race checks.

`TestStorageRefusesPublicDirectoriesAndSymlinkEscapes` intends to pass a public
0755 directory to the storage boundary. Mkdir applies the process umask; with
this workspace's 0077 the fixture became 0700 and the refusal assertion failed.
Explicitly chmod the fixture to 0755 before the assertion, making its intended
permissions independent of the shell. Its later 0700 and symlink checks remain.

`go test -race -count=1 ./internal/aicontext` passes after the fix. The original
full run continues through the remaining packages and will still report the
pre-fix failure. Record its outcome and rerun the full suite against the fixed
tree before claiming one clean full-suite pass. Keep the handoff current.
