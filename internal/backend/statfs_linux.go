//go:build linux

package backend

import "syscall"

// statfsUse is a folder's use and ceiling from the filesystem it is on. For a
// tmpfs the ceiling is the size the mount was given, which is the number a
// job's failure is measured against.
func statfsUse(path string) (used, size int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil || st.Bsize <= 0 {
		return 0, 0, false
	}
	bsize := int64(st.Bsize)
	size = int64(st.Blocks) * bsize
	used = size - int64(st.Bfree)*bsize
	if size <= 0 || used < 0 {
		return 0, 0, false
	}
	return used, size, true
}
