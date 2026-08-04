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

test('rulesets reject present-but-invalid constant and capability shapes', () => {
  const invalid = [
    { path: 'ability cap', apply: ruleset => { ruleset.constants.abilityCap = 'twenty'; } },
    { path: 'point buy', apply: ruleset => { ruleset.constants.pointBuy = 'broken'; } },
    { path: 'ASI', apply: ruleset => { ruleset.constants.asi.baseLevels = [0, 4]; } },
    { path: 'caster fractions', apply: ruleset => { ruleset.constants.casterFractions.half = 'sideways'; } },
    { path: 'pact magic', apply: ruleset => { ruleset.constants.pactMagic.tiers = [{ level: 1, slots: -1 }]; } },
    { path: 'rest', apply: ruleset => { ruleset.constants.rest.longRestHitDice = 'sometimes'; } },
    { path: 'capabilities', apply: ruleset => { ruleset.capabilities.weaponMastery = 'yes'; } },
  ];
  for (const fixture of invalid) {
    const ruleset = structuredClone(DEFAULT_RULESET);
    fixture.apply(ruleset);
    assert.equal(validateRuleset(ruleset).ok, false, fixture.path);
  }
});

test('accepted rulesets are detached snapshots', () => {
  const source = structuredClone(DEFAULT_RULESET);
  const inspected = inspectRulesDataHandle(handle({ ...makeFake(), getRuleset: () => source }));
  assert.equal(inspected.available, true);
  source.constants.abilityCap = 1;
  assert.equal(inspected.ruleset.constants.abilityCap, 20);
  assert.ok(Object.isFrozen(inspected.ruleset));
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
