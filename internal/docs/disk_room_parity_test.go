package docs

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/zoomies/internal/scheduler"
	"github.com/eyupio/zoomies/internal/store"
)

// The hint under a provider's Disk field does the provider check's arithmetic
// in the browser, from copies of two Go constants. A change to one side alone
// would have the form tell an operator a disk is enough and the check warn
// that it is not, so the copies are held to the originals here.
func TestTheDiskHintsFiguresMatchTheSchedulersAndTheStores(t *testing.T) {
	src, err := os.ReadFile("../../web/src/lib/providers/recommend.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		want int64
		from string
	}{
		{"RUNNER_DISK_MB", scheduler.RunnerDiskMB, "scheduler.RunnerDiskMB"},
		{"HOST_RESERVE_DISK_FLOOR_MB", store.MinHostReserveDiskMB, "store.MinHostReserveDiskMB"},
	} {
		got, ok := tsConstantProduct(string(src), tc.name)
		if !ok {
			t.Errorf("web/src/lib/providers/recommend.ts has no `export const %s = <whole number, or a product of them>;`", tc.name)
			continue
		}
		if got != tc.want {
			t.Errorf("web/src/lib/providers/recommend.ts sets %s to %d MB and %s is %d MB; change both together", tc.name, got, tc.from, tc.want)
		}
	}
}

// tsConstantProduct reads `export const NAME = 10 * 1024;` and the like: whole
// numbers multiplied together, which is all these constants are written as.
func tsConstantProduct(src, name string) (int64, bool) {
	m := regexp.MustCompile(`(?m)^export const ` + regexp.QuoteMeta(name) + `\s*=\s*([0-9_*\s]+);`).FindStringSubmatch(src)
	if m == nil {
		return 0, false
	}
	product := int64(1)
	for _, factor := range strings.Split(m[1], "*") {
		n, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(factor), "_", ""), 10, 64)
		if err != nil {
			return 0, false
		}
		product *= n
	}
	return product, true
}
