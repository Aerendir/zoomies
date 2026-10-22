package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// A pool row keeps ten documents in TEXT columns, and nearly all of them are
// what a column holds until somebody sets it: an empty object, an empty list, or
// the encoding of a type with nothing set. Those decode to the zero value and to
// nothing else, so reading a row does not hand them to the decoder, which was
// most of what reading a pool cost. These tests hold that shortcut to the
// decoder's own answer, for the documents it takes and for the ones that only
// look like them.

// scanPoolDecodingEverything is scanPool as it was before the shortcut: every
// document goes through encoding/json, and the columns that are lists and maps
// are read as text and decoded here, the way StringSlice and StringMap used to
// read them.
func scanPoolDecodingEverything(sc interface{ Scan(...any) error }) (*Pool, error) {
	var p Pool
	var idle, created, updated int64
	var ephemeral, runAsRoot, enabled, noDefaultLabels, sizeFromProfile, autoPaused int
	var labels, hostSelector, env, providerSelector string
	var resources, cache, runnerSettings, cpuBurst, tmpfs, memoryBurst string
	err := sc.Scan(&p.ID, &p.Name, &p.InstallationID, &labels, &p.RunnerGroup, &p.Backend,
		&p.Platform.OS, &p.Platform.OSVersion, &p.Platform.Arch,
		&p.Image, &p.PullPolicy, &p.RunnerVersion, &p.MinRunners, &p.MaxRunners, &p.Priority,
		&idle, &ephemeral, &p.DockerMode, &resources, &cache, &hostSelector, &env,
		&runAsRoot, &enabled, &created, &updated, &p.RepositoryScaleUpLimit, &p.CostPerRunnerHour,
		&runnerSettings, &cpuBurst, &noDefaultLabels, &sizeFromProfile, &tmpfs,
		&p.AutoKey, &p.AutoMin, &p.AutoCap, &autoPaused, &memoryBurst, &providerSelector, &p.DinDImage)
	if err != nil {
		return nil, err
	}
	p.NoDefaultLabels = noDefaultLabels == 1
	p.SizeFromProfile = sizeFromProfile == 1
	p.AutoPaused = autoPaused == 1
	p.IdleTimeout = Duration(time.Duration(idle) * time.Millisecond)
	p.Ephemeral, p.RunAsRoot, p.Enabled = ephemeral == 1, runAsRoot == 1, enabled == 1
	p.CreatedAt, p.UpdatedAt = at(created), at(updated)
	for _, d := range []struct {
		doc string
		out any
	}{
		{labels, &p.Labels}, {hostSelector, &p.HostSelector}, {env, &p.Env}, {providerSelector, &p.ProviderSelector},
		{resources, &p.Resources}, {cache, &p.Cache}, {runnerSettings, &p.RunnerSettings},
		{cpuBurst, &p.CPUBurst}, {tmpfs, &p.Tmpfs}, {memoryBurst, &p.MemoryBurst},
	} {
		if err := unmarshalJSON(d.doc, d.out); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

func listPoolsDecodingEverything(t *testing.T, s *Store) []*Pool {
	t.Helper()
	rows, err := s.read.QueryContext(context.Background(), `SELECT `+poolCols+` FROM pools ORDER BY name`)
	if err != nil {
		t.Fatalf("reading the pools: %v", err)
	}
	defer rows.Close()
	var out []*Pool
	for rows.Next() {
		p, err := scanPoolDecodingEverything(rows)
		if err != nil {
			t.Fatalf("decoding a pool: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the pools: %v", err)
	}
	return out
}

func TestPoolsReadBackAsTheDecoderAloneWouldReadThem(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	inst := &Installation{AppID: 1, InstallationID: 2, Target: "acme", TargetType: TargetOrg}
	if err := s.CreateInstallation(ctx, inst); err != nil {
		t.Fatalf("CreateInstallation: %v", err)
	}

	// Pools as they are made: nothing configured, and a good deal configured.
	n := 0
	for _, make := range []func(*Installation, int) *Pool{defaultBenchPool, configuredBenchPool} {
		n++
		if err := s.CreatePool(ctx, make(inst, n)); err != nil {
			t.Fatalf("CreatePool: %v", err)
		}
	}

	// And one pool for each document a column could hold that is not one of those:
	// the shapes the shortcut takes, and the ones that merely look like them.
	zeroCache, zeroTmpfs := zeroDoc[CacheConfig](), zeroDoc[TmpfsConfig]()
	structs := []string{"", "{}", " {} ", "{ }", "null", `{"unknown":1}`, `{"cpus":2}`}
	byColumn := map[string][]string{
		"resources":         structs,
		"runner_settings":   structs,
		"cpu_burst":         structs,
		"memory_burst":      structs,
		"cache":             append([]string{zeroCache, `{"scope":"","enabled":false}`, " " + zeroCache, `{"enabled":true}`}, structs...),
		"tmpfs":             append([]string{zeroTmpfs, " " + zeroTmpfs, `{"work":{"enabled":true}}`}, structs...),
		"labels":            {"", "[]", " [] ", "[ ]", "null", `["b","a"]`, `[""]`},
		"host_selector":     {"", "{}", " {} ", "{ }", "null", `{"a":"b"}`},
		"env":               {"", "{}", " {} ", "{ }", "null", `{"a":"b"}`},
		"provider_selector": {"", "{}", " {} ", "{ }", "null", `{"a":"b"}`},
	}
	for column, docs := range byColumn {
		for i, doc := range docs {
			n++
			p := defaultBenchPool(inst, n)
			p.Name = fmt.Sprintf("%s-%d", column, i)
			if err := s.CreatePool(ctx, p); err != nil {
				t.Fatalf("CreatePool: %v", err)
			}
			if _, err := s.write.ExecContext(ctx, `UPDATE pools SET `+column+` = ? WHERE id = ?`, doc, p.ID); err != nil {
				t.Fatalf("setting %s to %q: %v", column, doc, err)
			}
		}
	}

	want := listPoolsDecodingEverything(t, s)
	got, err := s.ListPools(ctx)
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}
	if len(got) != len(want) || len(want) != n {
		t.Fatalf("ListPools returned %d pools, the decoder alone %d, and %d were made", len(got), len(want), n)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("pool %s reads back as\n%+v\nwant what the decoder alone gives\n%+v", want[i].Name, got[i], want[i])
		}
		one, err := s.GetPool(ctx, want[i].ID)
		if err != nil || !reflect.DeepEqual(one, want[i]) {
			t.Errorf("GetPool(%s) = %+v, %v; want what the decoder alone gives\n%+v", want[i].Name, one, err, want[i])
		}
	}
}

// StringSlice and StringMap persist a list and a map in a TEXT column, in a
// good many tables. Scanning the empty one does not call the decoder, and has to
// leave exactly what the decoder would: an empty list that is not nil.
func TestScanningAnEmptyListOrMapGivesWhatTheDecoderGives(t *testing.T) {
	inputs := []any{
		nil, "", []byte(nil), []byte{}, "[]", []byte("[]"), "{}", []byte("{}"), " [] ", " {} ", "null", []byte("null"),
		`["a"]`, `{"a":"b"}`, `[]x`, `{}x`,
	}
	for _, in := range inputs {
		var gotSlice StringSlice
		var gotMap StringMap
		gotSliceErr, gotMapErr := gotSlice.Scan(in), gotMap.Scan(in)

		wantSlice, wantSliceErr := decodeAsBefore[StringSlice](in)
		wantMap, wantMapErr := decodeAsBefore[StringMap](in)

		if (gotSliceErr == nil) != (wantSliceErr == nil) || !reflect.DeepEqual(gotSlice, wantSlice) {
			t.Errorf("StringSlice.Scan(%#v) = %#v, %v; the decoder gives %#v, %v", in, gotSlice, gotSliceErr, wantSlice, wantSliceErr)
		}
		if (gotMapErr == nil) != (wantMapErr == nil) || !reflect.DeepEqual(gotMap, wantMap) {
			t.Errorf("StringMap.Scan(%#v) = %#v, %v; the decoder gives %#v, %v", in, gotMap, gotMapErr, wantMap, wantMapErr)
		}
	}
}

// decodeAsBefore is what StringSlice.Scan and StringMap.Scan did with every
// input before they knew the empty ones.
func decodeAsBefore[T any](in any) (T, error) {
	var out T
	switch t := in.(type) {
	case nil:
		return out, nil
	case []byte:
		if len(t) == 0 {
			return out, nil
		}
		err := json.Unmarshal(t, &out)
		return out, err
	case string:
		if t == "" {
			return out, nil
		}
		err := json.Unmarshal([]byte(t), &out)
		return out, err
	}
	return out, fmt.Errorf("cannot scan %T", in)
}

// The shortcut is only right while a type's zero value encodes to a document that
// decodes back to the zero value. Add a field whose zero does not, and this fails
// before a pool reads back wrong.
func TestTheDocumentAZeroPoolSettingEncodesToDecodesBackToIt(t *testing.T) {
	check := func(name string, doc string, decoded any, zero any) {
		t.Helper()
		if doc == "" {
			t.Errorf("%s: the zero value does not encode", name)
			return
		}
		if err := json.Unmarshal([]byte(doc), decoded); err != nil {
			t.Errorf("%s: %q does not decode: %v", name, doc, err)
			return
		}
		if !reflect.DeepEqual(reflect.ValueOf(decoded).Elem().Interface(), zero) {
			t.Errorf("%s: %q decodes to %+v, not to the zero value", name, doc, reflect.ValueOf(decoded).Elem().Interface())
		}
	}
	check("resources", zeroDoc[Resources](), new(Resources), Resources{})
	check("cache", zeroDoc[CacheConfig](), new(CacheConfig), CacheConfig{})
	check("runner settings", zeroDoc[RunnerSettings](), new(RunnerSettings), RunnerSettings{})
	check("CPU burst", zeroDoc[CPUBurstPolicy](), new(CPUBurstPolicy), CPUBurstPolicy{})
	check("tmpfs", zeroDoc[TmpfsConfig](), new(TmpfsConfig), TmpfsConfig{})
	check("memory burst", zeroDoc[MemoryBurstPolicy](), new(MemoryBurstPolicy), MemoryBurstPolicy{})
}
