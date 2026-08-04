import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeFake } from '../contract/synthetic-provider.mjs';
import {
  inspectRulesDataHandle,
  validateRuleset,
} from '../contract/rules-data.js';
import { DEFAULT_RULESET } from '../rules/ruleset.js';

const handle = (api, overrides = {}) => Object.freeze({
  api,
  provider: Object.freeze({
    addonId: 'synthetic-rules',
    addonName: 'Synthetic Rules',
    addonVersion: '1.0.0',
    contract: 'dnd5e.rules-data',
    contractVersion: '1.0.0',
    contentRevision: 'fixture-1',
    ...overrides,
  }),
});

test('rules-data contract accepts a complete synthetic provider and exposes identity', () => {
  const result = inspectRulesDataHandle(handle(makeFake()));
  assert.equal(result.available, true);
  assert.deepEqual(result.identity, {
    providerAddonId: 'synthetic-rules',
    providerAddonVersion: '1.0.0',
    providerContractVersion: '1.0.0',
    contentRevision: 'fixture-1',
    rulesetId: 'dnd-2024',
    rulesetVersion: 1,
    edition: '2024',
  });
});

test('rules-data contract rejects missing methods and incompatible service versions', () => {
  const missing = makeFake();
  delete missing.listSpells;
  assert.equal(inspectRulesDataHandle(handle(missing)).status, 'incompatible');
  assert.equal(inspectRulesDataHandle(handle(makeFake(), { contractVersion: '2.0.0' })).status, 'incompatible');
});

test('rulesets must be complete or explicitly inherit the supported base', () => {
  const incomplete = validateRuleset({
    rulesetId: 'other-edition',
    rulesetVersion: 1,
    edition: 'other',
    constants: { abilityCap: 18 },
  });
  assert.equal(incomplete.ok, false);
  assert.ok(incomplete.errors.some(error => error.includes('multiclassSlots')));

  const inherited = validateRuleset({
    rulesetId: 'dnd-2024-variant',
    rulesetVersion: 1,
    edition: '2024',
    extends: DEFAULT_RULESET.rulesetId,
    constants: { abilityCap: 18 },
  });
  assert.equal(inherited.ok, true);
  assert.equal(validateRuleset({ ...DEFAULT_RULESET, extends: 'unknown-base' }).ok, false);
});

test('loading and provider failures remain explicit unavailable states', () => {
  const pending = makeFake();
  pending.getRuleset = () => null;
  assert.equal(inspectRulesDataHandle(handle(pending)).status, 'pending');

  const failed = makeFake();
  failed.getRuleset = () => { throw new Error('broken data'); };
  const result = inspectRulesDataHandle(handle(failed));
  assert.equal(result.status, 'error');
  assert.deepEqual(result.errors, ['broken data']);
});
