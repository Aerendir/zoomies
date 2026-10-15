import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  preparesTemplates,
  templateIntro,
  templateSteps,
} from '../src/lib/providers/template-agent.ts';

// Preparing a template is Proxmox business: a provider of another kind has no
// template to prepare, and must not be offered the action.
test('only a provider whose template Zoomies prepares is offered the action', () => {
  assert.equal(preparesTemplates({ kind: 'proxmox' }), true);
  assert.equal(preparesTemplates({ kind: 'fake' }), false);
  assert.equal(preparesTemplates({}), false);
});

test('what a machine runs, and what changes it, is told once', () => {
  assert.match(templateIntro, /cloned from one template/);
  assert.match(templateIntro, /installs nothing of its own/);
  assert.match(templateIntro, /preparing the template again/);
});

test('the steps name the node, the same template, and the machines left alone', () => {
  const steps = templateSteps();
  const joined = steps.join(' ');
  assert.match(joined, /as root on the Proxmox node/);
  // The same templates, at the same VMID, so the provider row needs no change.
  assert.match(joined, /same VMID and the same name/);
  // The machines already running are replaced, not updated: a panel that
  // implied otherwise would have an operator wait for something that is not
  // coming.
  assert.match(joined, /replaced as they go idle/);
  assert.doesNotMatch(joined, /upgrade the machines|updates each machine/i);
  // The disk it replaced is named, with the command that removes it.
  assert.match(joined, /--delete unused0/);
  // The command it hands over is not a narrower thing than it is: it
  // reconciles the whole connection, and it ends by inviting the wizard, so
  // the steps say so and say not to follow that.
  assert.match(joined, /run again/);
  assert.match(joined, /private gateway/);
  assert.match(joined, /Do not add a provider afterwards/);
});
