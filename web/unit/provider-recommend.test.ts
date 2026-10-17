import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  DEFAULT_MACHINE,
  SUGGESTED_CEILING,
  machineSize,
  machinesPerNode,
  nodeReserveMb,
  recommendLimits,
} from '../src/lib/providers/recommend.ts';

const GB = 1024;

test('a node keeps a fifth of its memory for the hypervisor, and never less than two gigabytes', () => {
  assert.equal(nodeReserveMb(64 * GB), Math.round(64 * GB * 0.2));
  assert.equal(nodeReserveMb(8 * GB), 2 * GB);
  assert.ok(nodeReserveMb(2 * GB) <= GB, 'a tiny node still leaves something to place on');
});

test('memory is what fills a node first, and processors may be shared', () => {
  const node = { cpus: 16, memoryMb: 32 * GB };
  const size = { cpus: 2, memoryMb: 4 * GB, source: 'template' as const };
  // 32 GB less a fifth leaves 25.6 GB, which holds six of 4 GB; 16 cores shared
  // two to one would hold sixteen.
  assert.equal(machinesPerNode(node, size), 6);
  const heavy = { cpus: 8, memoryMb: 1 * GB, source: 'typed' as const };
  assert.equal(machinesPerNode(node, heavy), 4, 'processors bind when a machine asks for many');
});

test('the machine size is what was typed, then the template, then a default', () => {
  assert.deepEqual(machineSize({ cpus: '4', memoryMb: '8192' }, { cpus: 2, memory_mb: 4096 }), {
    cpus: 4,
    memoryMb: 8192,
    source: 'typed',
  });
  assert.deepEqual(machineSize({}, { cpus: 2, memory_mb: 4096 }), {
    cpus: 2,
    memoryMb: 4096,
    source: 'template',
  });
  assert.deepEqual(machineSize({}, null), DEFAULT_MACHINE);
});

test('a roomy node is suggested up to a first ceiling, with one machine warm', () => {
  const rec = recommendLimits([{ value: 'pve1', cpus: 32, memory_mb: 256 * GB }], {
    cpus: 2,
    memoryMb: 4 * GB,
    source: 'template',
  });
  assert.equal(rec.max, SUGGESTED_CEILING);
  assert.equal(rec.min, 1);
  assert.ok(rec.fits > rec.max, 'the node holds more than is suggested');
  assert.match(rec.reason, /first ceiling/);
});

test('the suggestion is what the nodes hold when that is below the ceiling', () => {
  const rec = recommendLimits(
    [
      { value: 'pve1', cpus: 8, memory_mb: 16 * GB },
      { value: 'pve2', cpus: 8, memory_mb: 16 * GB },
    ],
    { cpus: 2, memoryMb: 4 * GB, source: 'template' },
  );
  // 16 GB less 3.2 GB leaves 12.8 GB: three machines a node.
  assert.equal(rec.fits, 6);
  assert.equal(rec.max, 6);
  assert.match(rec.reason, /2 nodes/);
});

test('a node too small for one machine says so and suggests one, not none', () => {
  const rec = recommendLimits([{ value: 'pve1', cpus: 4, memory_mb: 4 * GB }], {
    cpus: 2,
    memoryMb: 4 * GB,
    source: 'template',
  });
  assert.equal(rec.fits, 0);
  assert.equal(rec.max, 1);
  assert.match(rec.reason, /cannot hold even one machine/);
});

// The sentence is what an operator reads under the box, so it has to say what
// the arithmetic does. A 4 GB node keeps 2 GB, which is half of it, and an 8 GB
// one keeps 2 GB, which is a quarter: "a fifth of its memory" described
// neither, and an operator who checked the sum against it would doubt the rest.
test('the reason says the larger of two gigabytes and a fifth is kept back, as the arithmetic does', () => {
  for (const memory of [3, 4, 8, 10, 64, 512]) {
    const kept = nodeReserveMb(memory * GB);
    assert.ok(kept >= 2 * GB, `${memory} GB keeps at least two gigabytes`);
    assert.ok(kept >= Math.round(memory * GB * 0.2), `${memory} GB keeps at least a fifth`);
    assert.ok(
      kept === 2 * GB || kept === Math.round(memory * GB * 0.2),
      `${memory} GB keeps one of the two, whichever is larger`,
    );
  }
  const size = { cpus: 2, memoryMb: 1 * GB, source: 'template' as const };
  for (const memory of [4, 8, 64]) {
    const rec = recommendLimits([{ value: 'pve1', cpus: 16, memory_mb: memory * GB }], size);
    assert.match(
      rec.reason,
      /after keeping memory for Proxmox \(at least 2 GB, or a fifth of its memory, whichever is larger\)/,
      `${memory} GB node`,
    );
    assert.doesNotMatch(rec.reason, /keeping a fifth of its memory/, `${memory} GB node`);
  }
});

test('two nodes are told each keeps the reserve, not that they share one', () => {
  const rec = recommendLimits(
    [
      { value: 'pve1', cpus: 8, memory_mb: 16 * GB },
      { value: 'pve2', cpus: 8, memory_mb: 16 * GB },
    ],
    { cpus: 2, memoryMb: 4 * GB, source: 'template' },
  );
  assert.match(rec.reason, /at least 2 GB, or a fifth of each node's memory, whichever is larger/);
});

test('a node too small after its reserve says what the reserve is', () => {
  const rec = recommendLimits([{ value: 'pve1', cpus: 4, memory_mb: 4 * GB }], {
    cpus: 2,
    memoryMb: 4 * GB,
    source: 'template',
  });
  assert.match(rec.reason, /cannot hold even one machine/);
  assert.match(rec.reason, /at least 2 GB, or a fifth of its memory, whichever is larger/);
});

// The figure is worked out from what a node has, not from what is free on it:
// the cluster is not asked what its other guests use. A node shared with other
// guests would be told it holds more than it does, so the sentence says where
// the figure comes from and which way a shared node differs.
test('the reason says the figure counts total memory, so a shared node holds fewer', () => {
  const rec = recommendLimits([{ value: 'pve1', cpus: 16, memory_mb: 32 * GB }], {
    cpus: 2,
    memoryMb: 4 * GB,
    source: 'template',
  });
  assert.match(
    rec.reason,
    /counts the node's total memory, not what other guests are using, so a node shared with other guests holds fewer/,
  );
});

test('a cluster that did not size its nodes gets a small start rather than a guess', () => {
  const rec = recommendLimits([{ value: 'pve1' }], DEFAULT_MACHINE);
  assert.deepEqual([rec.max, rec.min], [2, 1]);
  assert.match(rec.reason, /did not say how big/);
});

test('the minimum never exceeds the maximum', () => {
  for (const memory of [1, 2, 4, 8, 16, 64, 512]) {
    const rec = recommendLimits([{ value: 'n', cpus: 16, memory_mb: memory * GB }], {
      cpus: 2,
      memoryMb: 4 * GB,
      source: 'template',
    });
    assert.ok(rec.min <= rec.max, `${memory} GB: ${rec.min} warm of ${rec.max}`);
    assert.ok(rec.min >= 1);
  }
});
