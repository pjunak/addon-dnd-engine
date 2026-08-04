import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as Engine from '../rules/engine.js';
import { makeRulesApi } from '../rules/api.js';
import { DEFAULT_RULESET, resolveRuleset } from '../rules/ruleset.js';
import { makeFake } from '../contract/synthetic-provider.mjs';

const handle = (api = makeFake(), overrides = {}) => Object.freeze({
  api,
  provider: Object.freeze({
    addonId: 'fixture-provider',
    addonName: 'Fixture Provider',
    addonVersion: '2.3.4',
    contract: 'dnd5e.rules-data',
    contractVersion: '1.0.0',
    contentRevision: 'content-a',
    ...overrides,
  }),
});

test('provider-neutral arithmetic remains available without rules data', () => {
  const api = makeRulesApi(() => null);
  const result = api.hydrate({ abilities: { STR: 16, DEX: 14 }, level: 5 });
  assert.equal(result.sheet.abilities.STR.mod, 3);
  assert.equal(result.sheet.derived.proficiencyBonus, 3);
  assert.equal(result.sheet.derived.initiative, 2);
  assert.equal(api.getAvailability().available, false);
  assert.ok(result.warnings.some(warning => warning.includes('Rules data unavailable')));
});

test('public derive surface owns the sheet builder rule calculations', () => {
  const api = makeRulesApi(() => handle());
  assert.equal(api.derive.hitDieAverage('d10'), 6);
  assert.equal(api.derive.scrollCopyCost(3), 150);
  assert.equal(api.derive.pointBuyCost(15), 9);
  assert.equal(api.derive.pointsSpent({ STR: 15, DEX: 8, CON: 8, INT: 8, WIS: 8, CHA: 8 }), 9);
  assert.deepEqual(api.derive.featAsiFrom({ from: ['INT', 'WIS'], amount: 1 }), ['INT', 'WIS']);
  assert.equal(api.derive.featAbilityCap({ category: 'epicBoon' }), 30);
});

test('engine hydrates class, saves, hit points, spell slots, and spell DC from provider records', () => {
  const api = makeRulesApi(() => handle());
  const { sheet } = api.hydrate({ abilities: { INT: 16, CON: 14 }, className: 'Wizard', level: 5 });
  assert.equal(sheet.class.id, 'wizard');
  assert.equal(sheet.derived.hitDie, 'd6');
  assert.equal(sheet.derived.maxHp, 32);
  assert.equal(sheet.saves.INT.proficient, true);
  assert.deepEqual(sheet.spellcasting.slots, [4, 3, 2]);
  assert.equal(sheet.spellcasting.perClass[0].saveDC, 14);
});

test('pact magic remains a separate short-rest pool', () => {
  const api = makeRulesApi(() => handle());
  const { sheet } = api.hydrate({ classes: [{ classId: 'warlock', level: 5 }] });
  assert.deepEqual(sheet.spellcasting.slots, []);
  assert.deepEqual(sheet.spellcasting.perClass[0].pact, { slots: 2, level: 3 });
  const pool = sheet.resources.find(resource => resource.key === 'pact-slot');
  assert.equal(pool.max, 2);
  assert.equal(pool.recharge[0].on, 'short');
});

test('multiclass origin order and caster fractions are observable inputs', () => {
  const api = makeRulesApi(() => handle());
  const wizardFirst = api.hydrate({ classes: [
    { classId: 'wizard', level: 3 },
    { classId: 'fighter', level: 1 },
  ] }).sheet;
  const fighterFirst = api.hydrate({ classes: [
    { classId: 'fighter', level: 1 },
    { classId: 'wizard', level: 3 },
  ] }).sheet;
  assert.equal(wizardFirst.saves.INT.proficient, true);
  assert.equal(fighterFirst.saves.STR.proficient, true);

  const down = {
    ...DEFAULT_RULESET,
    rulesetId: 'dnd-other',
    id: 'dnd-other',
    edition: 'other',
    constants: {
      ...DEFAULT_RULESET.constants,
      casterFractions: { half: 'down', third: 'down' },
    },
  };
  const data = makeFake();
  data.getRuleset = () => down;
  const other = makeRulesApi(() => handle(data));
  assert.deepEqual(
    other.hydrate({ classes: [{ classId: 'paladin', level: 5 }, { classId: 'sorcerer', level: 1 }] }).sheet.spellcasting.slots,
    [4, 2],
  );
});

test('generic grants and active self-effects are interpreted without sourcebook IDs', () => {
  const api = makeRulesApi(() => handle());
  const { sheet } = api.hydrate({
    abilities: { STR: 16, DEX: 14, CON: 14 },
    className: 'Barbarian',
    level: 5,
    race: 'Dwarf',
    inventory: [{ ref: 'longsword', location: 'equipped' }],
  });
  assert.ok(sheet.resistances.includes('poison'));
  assert.equal(sheet.senses.darkvision, 120);
  assert.equal(sheet.derived.maxHp, 55);
  assert.ok(sheet.weapons.some(weapon => weapon.ref === 'longsword'));
});

test('public results and provider projections are detached from caller mutation', () => {
  const provider = makeFake();
  const api = makeRulesApi(() => handle(provider));
  const first = api.listClasses();
  first[0].name = 'Changed';
  assert.notEqual(api.listClasses()[0].name, 'Changed');

  const decisions = { abilities: { STR: 16 }, level: 1 };
  const result = api.hydrate(decisions);
  result.sheet.abilities.STR.score = 1;
  assert.equal(api.hydrate(decisions).sheet.abilities.STR.score, 16);
});

test('context identity carries provider, ruleset, contract, and content revision', () => {
  const api = makeRulesApi(() => handle());
  assert.deepEqual(api.getContextIdentity(), {
    engineContract: 'dnd5e.rules-engine',
    engineContractVersion: '1.0.0',
    rulesDataContract: 'dnd5e.rules-data',
    rulesDataContractVersion: '1.0.0',
    available: true,
    status: 'ready',
    providerAddonId: 'fixture-provider',
    providerAddonVersion: '2.3.4',
    providerContractVersion: '1.0.0',
    contentRevision: 'content-a',
    rulesetId: 'dnd-2024',
    rulesetVersion: 1,
    edition: '2024',
  });
});

test('ruleset inheritance is explicit and another edition receives no implicit 2024 merge', () => {
  const inherited = resolveRuleset({
    rulesetId: 'variant',
    rulesetVersion: 1,
    edition: '2024',
    extends: 'dnd-2024',
    constants: { abilityCap: 18 },
  });
  assert.equal(inherited.constants.abilityCap, 18);
  assert.ok(inherited.constants.multiclassSlots[20]);

  const other = resolveRuleset({
    rulesetId: 'other',
    rulesetVersion: 1,
    edition: 'other',
    constants: { abilityCap: 18 },
  });
  assert.equal(other.constants.multiclassSlots, undefined);
});

test('pure helpers stay deterministic and host-free', () => {
  assert.equal(Engine.abilityMod(9), -1);
  assert.equal(Engine.proficiencyBonus(17), 6);
  assert.deepEqual(Engine.multiclassSlots(5, DEFAULT_RULESET), [4, 3, 2]);
  const decisions = { abilities: { DEX: 14 }, level: 3 };
  assert.deepEqual(Engine.hydrate(decisions, null), Engine.hydrate(decisions, null));
});
