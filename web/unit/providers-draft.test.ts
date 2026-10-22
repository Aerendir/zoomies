import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  applyDiscovery,
  emptyDraft,
  draftErrors,
  savedNote,
  toProviderBody,
  normaliseEndpoint,
  suggestName,
  warmForMaximum,
  WIZARD_STEPS,
} from '../src/lib/providers/draft.ts';
import type { ProviderDiscovery, ProviderSetting } from '../src/lib/api/types.ts';

const specs: ProviderSetting[] = [
  {
    key: 'nodes',
    label: 'Nodes',
    kind: 'list',
    required: true,
    advanced: false,
    discovers: 'nodes',
  },
  {
    key: 'template_id',
    label: 'Template VMID',
    kind: 'choice',
    required: true,
    advanced: false,
    discovers: 'templates',
  },
  {
    key: 'bridge',
    label: 'Network bridge',
    kind: 'choice',
    required: true,
    advanced: false,
    discovers: 'bridges',
    default: 'vmbr0',
  },
  {
    key: 'storage',
    label: 'Storage',
    kind: 'choice',
    required: true,
    advanced: false,
    discovers: 'storages',
  },
];

// People type the host; the API wants an origin with a scheme and the port
// the driver listens on. Anything already complete is left alone.
test('an endpoint gains the scheme and the example port it was typed without', () => {
  const example = 'https://pve.example.com:8006';
  assert.equal(normaliseEndpoint('pve.home', example), 'https://pve.home:8006');
  assert.equal(normaliseEndpoint('https://pve.home', example), 'https://pve.home:8006');
  assert.equal(normaliseEndpoint('https://pve.home:8443', example), 'https://pve.home:8443');
  assert.equal(normaliseEndpoint('http://127.0.0.1:8006/', example), 'http://127.0.0.1:8006');
  assert.equal(normaliseEndpoint('  ', example), '');
  assert.equal(normaliseEndpoint('pve.home', undefined), 'https://pve.home');
  assert.equal(normaliseEndpoint('not a url at all', example), 'not a url at all');
});

test('a name is suggested from the host, and the kind stands in when there is no host', () => {
  assert.equal(suggestName('https://pve.example.com:8006', 'proxmox'), 'pve');
  assert.equal(suggestName('https://Lab-Cluster.local:8006', 'proxmox'), 'lab-cluster');
  assert.equal(suggestName('https://10.0.0.5:8006', 'proxmox'), 'proxmox');
  assert.equal(suggestName('', 'proxmox'), 'proxmox');
});

// Discovery fills in what is not a question -- one node, one template -- and
// replaces a published default the cluster does not have when there is one
// thing to replace it with. An answer the operator gave stays theirs.
test('discovery answers the questions with one answer and leaves the rest', () => {
  const draft = emptyDraft();
  draft.settings = { bridge: 'vmbr0', storage: 'local-lvm' };
  const discovery: ProviderDiscovery = {
    nodes: [{ value: 'pve1' }],
    templates: [{ value: '9000', label: 'zoomies-template' }],
    bridges: [{ value: 'vmbr1' }],
    storages: [{ value: 'ceph' }, { value: 'local-lvm' }],
  };
  const next = applyDiscovery(draft, specs, discovery);
  assert.equal(next.settings.nodes, 'pve1', 'the only node is chosen');
  assert.equal(next.settings.template_id, '9000', 'the only template is chosen');
  assert.equal(
    next.settings.bridge,
    'vmbr1',
    'a default the cluster lacks gives way to the one bridge it has',
  );
  assert.equal(next.settings.storage, 'local-lvm', 'a choice the operator made stays');

  const two: ProviderDiscovery = { ...discovery, nodes: [{ value: 'pve1' }, { value: 'pve2' }] };
  const open = applyDiscovery(emptyDraft(), specs, two);
  assert.equal(open.settings.nodes, undefined, 'two nodes is a question, not an answer');
  assert.equal(applyDiscovery(draft, specs, null), draft, 'no discovery changes nothing');
});

test('a ready Proxmox setup supplies the connection without sending secrets', () => {
  const draft = emptyDraft();
  draft.kind = 'proxmox';
  draft.setup_id = 'pvs_ready';
  draft.name = 'proxmox-pve-1';
  draft.endpoint = 'https://pve.example:8006';
  draft.connection = 'tailcat';
  draft.tailcat_configured = true;
  const errors = draftErrors(draft, []);
  assert.equal(errors.credential, undefined);
  assert.equal(errors.tailcat_address, undefined);
  const body = toProviderBody(draft);
  assert.equal(body.setup_id, 'pvs_ready');
  assert.equal(body.credential, undefined);
  assert.equal(body.tailcat_address, undefined);
  assert.equal(body.max_machines, 2);
  assert.equal(body.min_machines, 1);
});

// The box under Maximum says zero rents nothing. A draft opens on one warm
// machine, and a warm machine counts against the maximum, so a zero typed on its
// own was refused for a figure in the other box, with no word of why until that
// box was touched: "rents nothing" took two answers. It takes one now.
test('a maximum of zero takes the warm machine with it, so one answer rents nothing', () => {
  const draft = emptyDraft();
  draft.max_machines = '0';
  assert.ok(draftErrors(draft, []).min_machines, 'zero beside the opening one warm is refused');

  draft.min_machines = warmForMaximum(draft.max_machines, draft.min_machines);
  assert.equal(draft.min_machines, '0');
  assert.equal(draftErrors(draft, []).min_machines, undefined);
  assert.equal(draftErrors(draft, []).max_machines, undefined);
  assert.equal(toProviderBody(draft).max_machines, 0);
  assert.equal(toProviderBody(draft).min_machines, 0);
});

// Only a maximum that is zero changes the warm figure, and only when the
// maximum is what is being typed. A warm figure the operator typed after raising
// the maximum again is theirs, and no later keystroke in the other box moves it.
test('any other maximum leaves the warm figure the operator has alone', () => {
  assert.equal(warmForMaximum('3', '1'), '1');
  assert.equal(warmForMaximum('3', '2'), '2');
  assert.equal(warmForMaximum('1', '1'), '1');
  assert.equal(warmForMaximum('', '1'), '1', 'an empty box is not a zero');
  assert.equal(warmForMaximum('-1', '1'), '1', 'a wrong answer is left for the box to explain');
  assert.equal(warmForMaximum('0.5', '1'), '1');
  assert.equal(warmForMaximum('0', '0'), '0');
});

// A new provider opens on two and one, so words that say nothing is rented
// until a ceiling is raised describe a form that no longer exists.
test('the words about a ceiling of zero do not claim it is where a provider starts', () => {
  const limits = WIZARD_STEPS.find((step) => step.id === 'limits');
  assert.ok(limits?.description);
  assert.doesNotMatch(limits.description, /until the first of these is above zero/);
  assert.match(limits.description, /zero rents nothing/i);
  assert.equal(emptyDraft().max_machines, '2');
  assert.equal(emptyDraft().min_machines, '1');
});

// Saving creates nothing. What happens next is the next pass buying the warm
// machine, which is why the check is worth running now rather than before a
// first machine that nothing waits for.
test('saving says what happens next: the warm machine is bought on the next pass', () => {
  assert.match(savedNote(4, 1), /warm machine is bought on the next pass/);
  assert.match(savedNote(4, 1), /run the check/);
  assert.doesNotMatch(savedNote(4, 1), /before the first machine is built/);
});

test('saving a provider with no warm machine says machines wait for a pool that needs one', () => {
  assert.match(savedNote(4, 0), /rented when a pool has work with nowhere to run/);
  assert.doesNotMatch(savedNote(4, 0), /warm machine/);
});

test('saving a ceiling of zero says nothing is rented yet', () => {
  assert.match(savedNote(0, 0), /Nothing is rented yet/);
  assert.match(savedNote(0, 0), /ceiling is zero/);
});
