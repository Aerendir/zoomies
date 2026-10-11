//go:build !linux

package backend

// statfsUse measures nothing where there is no /proc to look through a
// container's root with: the Docker backend runs on Linux hosts, and an agent
// built for another system reports the folders unmeasured rather than empty.
func statfsUse(string) (used, size int64, ok bool) { return 0, 0, false }
