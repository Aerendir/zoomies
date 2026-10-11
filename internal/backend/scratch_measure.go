package backend

import (
	"path"
	"sort"
	"strconv"
)

// ScratchUse is one in-memory folder's use at a sample: the path it is mounted
// at in the container, what is written in it, and its ceiling.
//
// It exists because a folder that fills is the one failure of the setting a job
// cannot show afterwards. A compiler deletes its work directory as the link
// fails, and the runner's own output is the runner's diagnostics, never a
// step's "no space left on device"; by the time the job is reported failed the
// folder is empty again. So each sample measures the folders, and the fullest
// each got is kept on the job, where the explanation can read it.
type ScratchUse struct {
	Path      string `json:"path"`
	UsedBytes int64  `json:"used_bytes"`
	SizeBytes int64  `json:"size_bytes"`
}

// measureScratch reads each folder's use through the container's root, which
// the host's /proc exposes as /proc/<pid>/root, so no command runs inside the
// container and nothing is asked of the daemon. A folder statfs cannot answer
// for is left out rather than reported as empty: a containerised agent that
// does not share the host's pid namespace cannot see the root at all, and a
// made-up zero would read as a folder with room. No process is no folders.
//
// The paths are joined with forward slashes whatever the agent was built for:
// /proc and the mount paths inside the container are Linux paths, and an agent
// compiled elsewhere must still name the same folder it would on the host.
func measureScratch(pid int, paths []string, statfs func(path string) (used, size int64, ok bool)) []ScratchUse {
	if pid <= 0 || len(paths) == 0 {
		return nil
	}
	root := path.Join("/proc", strconv.Itoa(pid), "root")
	var out []ScratchUse
	for _, p := range paths {
		used, size, ok := statfs(path.Join(root, p))
		if !ok || size <= 0 {
			continue
		}
		out = append(out, ScratchUse{Path: p, UsedBytes: used, SizeBytes: size})
	}
	return out
}

// scratchOf is the use of every tmpfs an inspected container was created
// with, in the order of their paths, so a sample reads the same way twice.
// Nothing for a container that could not be inspected or has no process.
func scratchOf(insp *ContainerInspect, err error) []ScratchUse {
	if err != nil || insp == nil || insp.State == nil || insp.HostConfig == nil || len(insp.HostConfig.Tmpfs) == 0 {
		return nil
	}
	paths := make([]string, 0, len(insp.HostConfig.Tmpfs))
	for p := range insp.HostConfig.Tmpfs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return measureScratch(insp.State.Pid, paths, statfsUse)
}
