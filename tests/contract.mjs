import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeFake } from '../contract/synthetic-provider.mjs';
import {
  inspectRulesDataHandle,
  validateRuleset,
} from '../contract/rules-data.js';
import {
  SYNTHETIC_2014_RULESET,
  SYNTHETIC_2024_RULESET,
} from '../contract/synthetic-rulesets.mjs';

const handle = (api, overrides = {}) => Object.freeze({
  api,
  provider: Object.freeze({
    addonId: 'synthetic-rules',
    addonName: 'Synthetic Rules',
    addonVersion: '1.0.0',
    contract: 'dnd5e.rules-data',
    contractVersion: '2.0.0',
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
    providerContractVersion: '2.0.0',
    contentRevision: 'fixture-1',
    rulesetId: 'synthetic-dnd-2024',
    rulesetVersion: 2,
    edition: '2024',
  });
});

test('rules-data contract rejects missing methods and incompatible service versions', () => {
  const missing = makeFake();
  delete missing.listSpells;
  assert.equal(inspectRulesDataHandle(handle(missing)).status, 'incompatible');
  assert.equal(inspectRulesDataHandle(handle(makeFake(), { contractVersion: '1.0.0' })).status, 'incompatible');
});

test('rulesets must be complete and cannot inherit an engine-owned base', () => {
  const incomplete = validateRuleset({
    rulesetId: 'other-edition',
    rulesetVersion: 1,
    edition: 'other',
    constants: { abilityCap: 18 },
  });
  assert.equal(incomplete.ok, false);
  assert.ok(incomplete.errors.some(error => error.includes('multiclassSlots')));

  assert.equal(validateRuleset({ ...SYNTHETIC_2024_RULESET, extends: 'engine-default' }).ok, false);
  assert.equal(validateRuleset(SYNTHETIC_2014_RULESET).ok, true);
});

test('rulesets reject present-but-invalid constant and capability shapes', () => {
  const invalid = [
    { path: 'ability cap', apply: ruleset => { ruleset.constants.abilityCap = 'twenty'; } },
    { path: 'point buy', apply: ruleset => { ruleset.constants.pointBuy = 'broken'; } },
    { path: 'advancement', apply: ruleset => { ruleset.builder.abilityScoreAdvancement.baseLevels = [0, 4]; } },
    { path: 'caster fractions', apply: ruleset => { ruleset.constants.casterFractions.half = 'sideways'; } },
    { path: 'pact magic', apply: ruleset => { ruleset.constants.pactMagic.tiers = [{ level: 1, slots: -1 }]; } },
    { path: 'rest', apply: ruleset => { ruleset.constants.rest.longRestHitDice = 'sometimes'; } },
    { path: 'capabilities', apply: ruleset => { ruleset.capabilities.weaponMastery = 'yes'; } },
    { path: 'origin ability grants', apply: ruleset => { ruleset.builder.backgroundAbilityGrant = { budget: 3, perAbilityMax: 4 }; } },
  ];
  for (const fixture of invalid) {
    const ruleset = structuredClone(SYNTHETIC_2024_RULESET);
    fixture.apply(ruleset);
    assert.equal(validateRuleset(ruleset).ok, false, fixture.path);
  }
});

test('accepted rulesets are detached snapshots', () => {
  const source = structuredClone(SYNTHETIC_2024_RULESET);
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
