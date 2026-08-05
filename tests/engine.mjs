import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as Engine from '../rules/engine.js';
import { makeRulesApi } from '../rules/api.js';
import { requireRuleset } from '../rules/ruleset.js';
import { makeFake } from '../contract/synthetic-provider.mjs';
import {
  SYNTHETIC_2014_RULESET,
  SYNTHETIC_2024_RULESET,
} from '../contract/synthetic-rulesets.mjs';

const handle = (api = makeFake(), overrides = {}) => Object.freeze({
  api,
  provider: Object.freeze({
    addonId: 'fixture-provider',
    addonName: 'Fixture Provider',
    addonVersion: '2.3.4',
    contract: 'dnd5e.rules-data',
    contractVersion: '2.0.0',
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

  const data = makeFake();
  data.getRuleset = () => SYNTHETIC_2014_RULESET;
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

test('malformed provider results fail closed at the public engine boundary', () => {
  const provider = makeFake();
  provider.listClasses = () => ({ not: 'an array' });
  provider.getItem = () => [];
  provider.getItemByName = () => { throw new Error('provider failure'); };
  const api = makeRulesApi(() => handle(provider));
  assert.deepEqual(api.listClasses(), []);
  assert.equal(api.getItem('class', 'wizard'), null);
  assert.doesNotThrow(() => api.hydrate({ className: 'Wizard', level: 1 }));
});

test('context identity carries provider, ruleset, contract, and content revision', () => {
  const api = makeRulesApi(() => handle());
  assert.deepEqual(api.getContextIdentity(), {
    engineContract: 'dnd5e.rules-engine',
    engineContractVersion: '2.0.0',
    rulesDataContract: 'dnd5e.rules-data',
    rulesDataContractVersion: '2.0.0',
    available: true,
    status: 'ready',
    providerAddonId: 'fixture-provider',
    providerAddonVersion: '2.3.4',
    providerContractVersion: '2.0.0',
    contentRevision: 'content-a',
    rulesetId: 'synthetic-dnd-2024',
    rulesetVersion: 2,
    edition: '2024',
  });
});

test('engine has no native rules revision and requires a provider profile', () => {
  assert.throws(() => requireRuleset(), /required/);
  assert.equal(requireRuleset(SYNTHETIC_2014_RULESET).edition, '2014');
  assert.equal(requireRuleset(SYNTHETIC_2024_RULESET).edition, '2024');
});

test('builder plan normalizes edition-specific origin and advancement policy', () => {
  const current = makeRulesApi(() => handle());
  const plan2024 = current.getBuilderPlan({
    background: 'Acolyte',
    classes: [{ classId: 'fighter', level: 19 }],
  });
  assert.deepEqual(plan2024.creationAbilityChoices.map(choice => choice.id), ['bgasi']);
  assert.deepEqual(
    plan2024.classChoices.find(choice => choice.id === 'asi:fighter:19').feat.categories,
    ['general', 'epicBoon'],
  );
  assert.ok(plan2024.classChoices.some(choice => choice.id === 'asi:fighter:14'));

  const provider2014 = makeFake();
  provider2014.getRuleset = () => SYNTHETIC_2014_RULESET;
  const legacy = makeRulesApi(() => handle(provider2014));
  const plan2014 = legacy.getBuilderPlan({
    species: 'Dwarf',
    classes: [{ classId: 'fighter', level: 19 }],
  });
  assert.deepEqual(plan2014.creationAbilityChoices.map(choice => choice.id), ['speciesasi']);
  assert.deepEqual(
    plan2014.classChoices.find(choice => choice.id === 'asi:fighter:19').feat.categories,
    ['general'],
  );
});

test('builder mutations apply normalized ability budgets and feat caps', () => {
  const api = makeRulesApi(() => handle());
  let decisions = {
    background: 'Acolyte',
    classes: [{ classId: 'fighter', level: 19 }],
    featureChoices: {},
    abilityGrants: [],
  };
  decisions = api.applyBuilderChoice(decisions, {
    choiceId: 'bgasi',
    value: { ability: 'INT', amount: 3 },
  });
  assert.deepEqual(decisions.abilityGrants[0].assign, { INT: 2 });

  decisions = api.applyBuilderChoice(decisions, { choiceId: 'asi:fighter:19', value: 'feat' });
  decisions = api.applyBuilderChoice(decisions, { choiceId: 'asi:fighter:19:feat', value: 'boon-of-fortitude' });
  assert.deepEqual(
    decisions.abilityGrants.find(grant => grant.id === 'asi:fighter:19:featability'),
    {
      id: 'asi:fighter:19:featability',
      source: { type: 'feat' },
      assign: { CON: 1 },
      cap: 30,
    },
  );
});

test('pure helpers stay deterministic and host-free', () => {
  assert.equal(Engine.abilityMod(9), -1);
  assert.equal(Engine.proficiencyBonus(17), 6);
  assert.deepEqual(Engine.multiclassSlots(5, SYNTHETIC_2024_RULESET), [4, 3, 2]);
  const decisions = { abilities: { DEX: 14 }, level: 3 };
  assert.deepEqual(
    Engine.hydrate(decisions, null, SYNTHETIC_2024_RULESET),
    Engine.hydrate(decisions, null, SYNTHETIC_2024_RULESET),
  );
});
