import { test } from 'node:test';
import assert from 'node:assert/strict';
import { hostVitals, type Vital, type VitalKey, type Vitals } from '../src/lib/hosts/vitals.ts';
import { NO_VALUE } from '../src/lib/format.ts';
import type { Host } from '../src/lib/api/types.ts';

const NOW = Date.parse('2026-10-17T10:00:00Z');
const GIB = 1024;
const TIB = 1024 * GIB;

// A host that reports everything and has room to spare, built to the generated
// type. Each test changes only what it is about.
function host(over: Partial<Host> = {}): Host {
  return {
    id: 'host_1',
    name: 'builder-1',
    healthy: true,
    resources_known: true,
    cpus: 8,
    memory_mb: 16 * GIB,
    // What the scheduler may place: the machine less the 819 MB (5%) a host of
    // this size always holds back. The same figure the demo fleet reports.
    allocatable_memory_mb: 16 * GIB - 819,
    disk_total_mb: 220 * GIB,
    disk_free_mb: 62 * GIB,
    usage_fresh: true,
    usage: { cpu_percent: 34, memory_available_mb: 9830, sampled_at: '2026-10-17T09:59:50Z' },
    ...over,
  };
}

function vital(vitals: Vitals, key: VitalKey): Vital {
  const found = vitals.items.find((item) => item.key === key);
  assert.ok(found, `${key} is always in the strip, measured or not`);
  return found;
}

test('a healthy host reports CPU, memory and disk as shares of the machine, in that order', () => {
  // The card and the capacity map must give one answer for one host, so these
  // are the shares hostSeries.ts computes: memory and disk are used, not free.
  const vitals = hostVitals(host(), NOW);
  assert.deepEqual(
    vitals.items.map((item) => item.key),
    ['cpu', 'memory', 'disk'],
  );
  const cpu = vital(vitals, 'cpu');
  assert.equal(cpu.value, '34%');
  assert.equal(cpu.detail, '8 vCPU');
  const memory = vital(vitals, 'memory');
  assert.equal(memory.value, '40%', '6554 of 16384 MB');
  assert.equal(memory.detail, '6.4 / 16 GB');
  const disk = vital(vitals, 'disk');
  assert.equal(disk.value, '72%', '158 of 220 GiB');
  assert.equal(disk.detail, '158 / 220 GB');
  assert.deepEqual(
    vitals.items.map((item) => item.tight),
    [false, false, false],
  );
  assert.equal(vitals.unavailable, '', 'something is measured, so nothing needs explaining');
});

test('CPU turns tight at the scheduler pressure line, counted without the CPU lent to runners', () => {
  // From 85% the scheduler places one runner at a time, but it judges CPU
  // without what elastic CPU lent to runners: that is load the plan chose, and
  // counting it would have a boost trip the very limit that then withdraws it
  // (hostUnderCPUPressure reads UnlentCPUPercent). ResourceBar's 90% is a
  // different rule.
  const cases: {
    name: string;
    cpu: number;
    lent?: number;
    value: string;
    tight: boolean;
  }[] = [
    { name: 'at the line', cpu: 85, value: '85%', tight: true },
    { name: 'a point under', cpu: 84, value: '84%', tight: false },
    { name: 'busy, with 30 of it lent to runners', cpu: 92, lent: 30, value: '92%', tight: false },
    { name: 'busy, with only 5 of it lent', cpu: 92, lent: 5, value: '92%', tight: true },
    { name: 'at the line, with a point of it lent', cpu: 85, lent: 1, value: '85%', tight: false },
  ];
  for (const c of cases) {
    const usage: Host['usage'] = { cpu_percent: c.cpu, memory_available_mb: 9830 };
    if (c.lent !== undefined) usage.lent_cpu_percent = c.lent;
    const found = vital(hostVitals(host({ usage }), NOW), 'cpu');
    assert.equal(found.value, c.value, c.name);
    assert.equal(found.tight, c.tight, c.name);
  }
});

test('a busy CPU that is mostly lent says why it is not amber', () => {
  // A bar at 92% that is not amber looks like a bug unless it says what is
  // excluded, and the capacity map beside it still draws the raw figure.
  const lent = vital(
    hostVitals(
      host({ usage: { cpu_percent: 92, memory_available_mb: 9830, lent_cpu_percent: 30 } }),
      NOW,
    ),
    'cpu',
  );
  assert.match(lent.summary, /30% of it is CPU lent to runners/);
  assert.doesNotMatch(lent.summary, /one runner at a time/, 'it is not under pressure');
  const pressed = vital(
    hostVitals(
      host({ usage: { cpu_percent: 92, memory_available_mb: 9830, lent_cpu_percent: 5 } }),
      NOW,
    ),
    'cpu',
  );
  assert.match(pressed.summary, /one runner at a time/);
  assert.match(pressed.summary, /not lent/, 'the line it passed leaves lent CPU out');
});

test('memory is tight at the host reserve the scheduler holds starts at, not at 85% used', () => {
  // The scheduler has no pressure line for memory. It holds new starts when
  // available memory is at or below the host's reserve (HostAdmissionReason),
  // which is 5% of the machine with a 512 MB floor, and a gigabyte more on the
  // host the controller itself runs on. A 16 GiB host at 94% used has 983 MB
  // left against a reserve of 819 MB and places as normal.
  const cases: {
    name: string;
    over: Partial<Host>;
    available: number;
    value: string;
    tight: boolean;
  }[] = [
    { name: '94% used, above the reserve', over: {}, available: 983, value: '94%', tight: false },
    { name: 'a megabyte above the reserve', over: {}, available: 820, value: '95%', tight: false },
    { name: 'exactly at the reserve', over: {}, available: 819, value: '95%', tight: true },
    { name: 'under the reserve', over: {}, available: 100, value: '99%', tight: true },
    {
      // memory_mb less allocatable_memory_mb is the reserve, floors included,
      // whoever set it, so the card never has to know the floors.
      name: 'a reserve the operator raised to 4 GB',
      over: { allocatable_memory_mb: 16 * GIB - 4096 },
      available: 4096,
      value: '75%',
      tight: true,
    },
    {
      name: 'the same host just above its raised reserve',
      over: { allocatable_memory_mb: 16 * GIB - 4096 },
      available: 4097,
      value: '75%',
      tight: false,
    },
    {
      name: 'the controller host, which keeps a gigabyte more back',
      over: { allocatable_memory_mb: 16 * GIB - 1843 },
      available: 1843,
      value: '89%',
      tight: true,
    },
    {
      // A reserve at or past the machine leaves nothing to place on, and the
      // API leaves allocatable_memory_mb out rather than send a zero.
      name: 'a reserve that covers the whole machine',
      over: { allocatable_memory_mb: undefined },
      available: 9830,
      value: '40%',
      tight: true,
    },
  ];
  for (const c of cases) {
    const usage = { cpu_percent: 10, memory_available_mb: c.available };
    const found = vital(hostVitals(host({ ...c.over, usage }), NOW), 'memory');
    assert.equal(found.value, c.value, c.name);
    assert.equal(found.tight, c.tight, c.name);
  }
});

test('a memory figure at the reserve says it is the reserve, not a pressure line', () => {
  const at = vital(
    hostVitals(host({ usage: { cpu_percent: 10, memory_available_mb: 819 } }), NOW),
    'memory',
  );
  assert.match(at.summary, /at or below the 819 MB/);
  assert.match(at.summary, /holds new starts/);
  assert.doesNotMatch(at.summary, /85%|one runner at a time/, 'that is the CPU rule');
});

test('disk is tight by the doctor rule for a low disk, not by the pressure line', () => {
  // The amber figure and the Work directory free space warning must agree
  // (diskIsLow in health-card.ts): under 10% or under 10 GiB free. A share of
  // 85% would flag the big disk below and miss the small one.
  const cases: { name: string; total: number; free: number; tight: boolean }[] = [
    { name: 'a small disk with under 10 GiB left', total: 40 * GIB, free: 9 * GIB, tight: true },
    {
      name: 'a big disk 85% full with 150 GiB left',
      total: 1000 * GIB,
      free: 150 * GIB,
      tight: false,
    },
    { name: 'exactly 10% free is not yet low', total: 200 * GIB, free: 20 * GIB, tight: false },
    { name: 'a hair under 10% free is low', total: 200 * GIB, free: 20 * GIB - 1, tight: true },
  ];
  for (const c of cases) {
    const disk = vital(
      hostVitals(host({ disk_total_mb: c.total, disk_free_mb: c.free }), NOW),
      'disk',
    );
    assert.equal(disk.tight, c.tight, c.name);
  }
});

test('a measured zero is a figure, not a gap', () => {
  // memory_available_mb of 0 is a measured full host (the API says so), and an
  // idle CPU reads 0. Treating either as missing would show a dash on exactly
  // the machines that most need looking at, or on the quietest.
  const vitals = hostVitals(
    host({
      usage: { cpu_percent: 0, memory_available_mb: 0 },
      disk_free_mb: 0,
    }),
    NOW,
  );
  assert.equal(vital(vitals, 'cpu').value, '0%');
  assert.equal(vital(vitals, 'cpu').percent, 0);
  assert.equal(vital(vitals, 'memory').value, '100%');
  assert.equal(vital(vitals, 'memory').tight, true);
  assert.equal(vital(vitals, 'disk').value, '100%');
  assert.equal(vital(vitals, 'disk').tight, true);
});

test('stale usage leaves CPU and memory unmeasured while disk and the machine size stay', () => {
  // A reading that has gone stale is not shown as current: the controller says
  // so with usage_fresh. The size of the machine does not go stale, and the
  // disk is reported separately.
  const vitals = hostVitals(host({ usage_fresh: false }), NOW);
  const cpu = vital(vitals, 'cpu');
  assert.equal(cpu.percent, null);
  assert.equal(cpu.value, NO_VALUE);
  assert.equal(cpu.detail, '8 vCPU');
  assert.equal(cpu.tight, false);
  const memory = vital(vitals, 'memory');
  assert.equal(memory.value, NO_VALUE);
  assert.equal(memory.detail, '16 GB', 'the total alone, without a used figure to pair it with');
  assert.equal(vital(vitals, 'disk').value, '72%');
  assert.equal(vitals.unavailable, '', 'the disk is still measured');
});

test('a host that has stopped sending heartbeats shows no figure as current, disk included', () => {
  // The disk figure is whatever the agent last said, and a card that showed it
  // beside "No heartbeat since 20 minutes ago" would present it as now.
  const vitals = hostVitals(host({ healthy: false }), NOW);
  for (const item of vitals.items) {
    assert.equal(item.percent, null, item.key);
    assert.equal(item.value, NO_VALUE, item.key);
    assert.equal(item.tight, false, item.key);
  }
  assert.match(vitals.unavailable, /heartbeat/i, 'the reason names the cause');
});

test('an agent that reports neither usage nor its machine is explained once, not dashed three times', () => {
  const vitals = hostVitals(
    host({
      resources_known: false,
      cpus: undefined,
      memory_mb: undefined,
      disk_total_mb: undefined,
      disk_free_mb: undefined,
      usage: undefined,
      usage_fresh: false,
    }),
    NOW,
  );
  for (const item of vitals.items) {
    assert.equal(item.percent, null, item.key);
    assert.equal(item.detail, '', `${item.key} has no size to name`);
  }
  assert.notEqual(
    vitals.unavailable,
    '',
    'without a sentence the card is three dashes and no reason',
  );
});

test('sizes of a terabyte or more are written in terabytes so the pair stays short', () => {
  // The figure sits in a narrow column; "1,024 / 2,048 GB" would not fit it.
  const disk = vital(
    hostVitals(host({ disk_total_mb: 2 * TIB, disk_free_mb: 1 * TIB }), NOW),
    'disk',
  );
  assert.equal(disk.detail, '1 / 2 TB');
  assert.equal(disk.value, '50%');
});

test('a figure past its line says so in words as well as colour', () => {
  // Amber alone is not an answer for someone who cannot see it, so the
  // sentence read aloud and shown in the tooltip names the line that was passed.
  const busy = hostVitals(host({ usage: { cpu_percent: 91, memory_available_mb: 9830 } }), NOW);
  assert.match(vital(busy, 'cpu').summary, /85%/);
  assert.doesNotMatch(vital(busy, 'memory').summary, /85%/, 'only the figure past the line');
  const low = hostVitals(host({ disk_total_mb: 40 * GIB, disk_free_mb: 9 * GIB }), NOW);
  assert.match(vital(low, 'disk').summary, /low/i);
  for (const item of hostVitals(host(), NOW).items) {
    assert.ok(item.summary.startsWith(item.label), `${item.key} leads with its own name`);
  }
});
