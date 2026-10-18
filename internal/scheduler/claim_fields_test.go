package scheduler

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/store"
)

// A claim does not need a whole pool. The controller reads the fields
// store.ClaimView keeps, and no others, to decide which pool takes a job, because
// reading the rest of every row was most of what a webhook cost at a hundred
// pools. This holds BestPool to that: each pool is filled in everywhere
// ClaimView lets go of it, with values no two pools share, and the job must land
// where it lands when those fields are gone. A choice that began to depend on,
// say, MaxRunners would come out differently here than it does for the
// controller, which never reads it, and this is where that would be found.

func TestChoosingAPoolDependsOnNothingAClaimViewLetsGoOf(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261019, 7))
	routed, fromHosts, tied := 0, 0, 0
	for i := range 5000 {
		c := randomMatchingCase(rng)
		// Half the fleets fit the job equally well in every pool, so that the
		// choice between them is nothing but tie-break.
		if rng.IntN(2) == 0 {
			if tieFleet(&c) >= 2 {
				tied++
			}
		}
		// Routing by size class and the pools the controller keeps are what
		// lessPoolFor reads beyond a name, so some cases have them.
		if rng.IntN(3) == 0 {
			c.job.RoutedClass = store.SizeClasses()[rng.IntN(3)]
			routed++
		}
		for _, p := range c.pools {
			if p != nil && rng.IntN(3) == 0 {
				p.AutoKey = store.AutoKeyFor([]string{"amd64", "arm64"}[rng.IntN(2)], store.SizeClasses()[rng.IntN(3)])
				p.Platform.Arch = []string{"amd64", "arm64"}[rng.IntN(2)]
				fromHosts++
			}
		}

		whole := make([]*store.Pool, len(c.pools))
		view := make([]*store.Pool, len(c.pools))
		for k, p := range c.pools {
			if p != nil {
				whole[k] = withEverythingElse(p, k+1)
				view[k] = store.ClaimView(whole[k])
			}
		}
		wantPool, wantWhy := bestPool(whole, c.job, c.targets)
		gotPool, gotWhy := bestPool(view, c.job, c.targets)
		if indexIn(view, gotPool) != indexIn(whole, wantPool) || gotWhy != wantWhy {
			t.Fatalf("case %d: the claim views chose pool %d (%q), the whole pools pool %d (%q)\njob labels %q on installation %q routed to %q\npools:\n%s",
				i, indexIn(view, gotPool), gotWhy, indexIn(whole, wantPool), wantWhy,
				c.job.Labels, c.job.InstallationID, c.job.RoutedClass, describePools(whole))
		}
	}
	if routed < 1000 || fromHosts < 1000 || tied < 1000 {
		t.Fatalf("only %d cases were routed by class, %d pools were kept from hosts and %d fleets tied; the comparison needs all three", routed, fromHosts, tied)
	}
}

// tieFleet makes every pool fit the job exactly as well as the others, and
// returns how many pools there are to choose between.
func tieFleet(c *matchingCase) int {
	c.job.InstallationID = "ins_a"
	c.job.Labels = store.StringSlice{"gpu"}
	n := 0
	for _, p := range c.pools {
		if p != nil {
			p.InstallationID, p.Enabled, p.NoDefaultLabels = "ins_a", true, false
			p.Labels = store.StringSlice{"gpu", "linux"}
			n++
		}
	}
	return n
}

// indexIn is where a pool is among pools, or -1.
func indexIn(pools []*store.Pool, p *store.Pool) int {
	for i, q := range pools {
		if q != nil && q == p {
			return i
		}
	}
	return -1
}

// withEverythingElse is a copy of p with every field ClaimView lets go of, that p
// leaves unset, set to a value of its own. Which fields those are is found, not
// listed: a pool is filled in everywhere, and what its claim view lacks is what
// is let go. A value p sets itself on such a field is kept, so that a field the
// view should have kept, and a claim reads, is still there to be missed.
func withEverythingElse(p *store.Pool, seed int) *store.Pool {
	filled := &store.Pool{}
	fill(reflect.ValueOf(filled).Elem(), seed)
	kept := store.ClaimView(filled)

	out := *p
	letGo(reflect.ValueOf(&out).Elem(), reflect.ValueOf(filled).Elem(), reflect.ValueOf(kept).Elem())
	return &out
}

var timeType = reflect.TypeOf(time.Time{})

// fill sets every settable field under v to a value that is not the zero one.
func fill(v reflect.Value, seed int) {
	switch v.Kind() {
	case reflect.String:
		v.SetString(fmt.Sprintf("v%d", seed))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(seed))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(seed))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(seed) + 0.5)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0), seed)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		fill(k, seed)
		fill(e, seed)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.Pointer:
		n := reflect.New(v.Type().Elem())
		fill(n.Elem(), seed)
		v.Set(n)
	case reflect.Struct:
		if v.Type() == timeType {
			v.Set(reflect.ValueOf(time.Unix(int64(seed), 0).UTC()))
			return
		}
		for i := range v.NumField() {
			if f := v.Field(i); f.CanSet() {
				fill(f, seed)
			}
		}
	}
}

// letGo copies into dst each field of filled that kept does not have, where dst
// has none of its own.
func letGo(dst, filled, kept reflect.Value) {
	for i := range dst.NumField() {
		d, f, k := dst.Field(i), filled.Field(i), kept.Field(i)
		if !d.CanSet() || !f.CanInterface() {
			continue
		}
		switch {
		case f.Kind() == reflect.Struct && f.Type() != timeType:
			letGo(d, f, k)
		case d.IsZero() && !reflect.DeepEqual(f.Interface(), k.Interface()):
			d.Set(f)
		}
	}
}
