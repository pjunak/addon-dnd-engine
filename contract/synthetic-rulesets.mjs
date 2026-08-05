// Redistributable complete profiles used only by conformance tests. Production
// edition policy belongs to rules-data providers, never to the engine package.

const MULTICLASS_SLOTS = Object.freeze({
  1: [2], 2: [3], 3: [4, 2], 4: [4, 3], 5: [4, 3, 2], 6: [4, 3, 3], 7: [4, 3, 3, 1],
  8: [4, 3, 3, 2], 9: [4, 3, 3, 3, 1], 10: [4, 3, 3, 3, 2], 11: [4, 3, 3, 3, 2, 1],
  12: [4, 3, 3, 3, 2, 1], 13: [4, 3, 3, 3, 2, 1, 1], 14: [4, 3, 3, 3, 2, 1, 1],
  15: [4, 3, 3, 3, 2, 1, 1, 1], 16: [4, 3, 3, 3, 2, 1, 1, 1], 17: [4, 3, 3, 3, 2, 1, 1, 1, 1],
  18: [4, 3, 3, 3, 3, 1, 1, 1, 1], 19: [4, 3, 3, 3, 3, 2, 1, 1, 1], 20: [4, 3, 3, 3, 3, 2, 2, 1, 1],
});

const COMMON_CONSTANTS = Object.freeze({
  abilityCap: 20,
  abilityCapHard: 30,
  attunementLimit: 3,
  scrollCopyGpPerLevel: 50,
  pointBuy: Object.freeze({
    budget: 27,
    min: 8,
    max: 15,
    cost: Object.freeze({ 8: 0, 9: 1, 10: 2, 11: 3, 12: 4, 13: 5, 14: 7, 15: 9 }),
  }),
  multiclassSlots: MULTICLASS_SLOTS,
  pactMagic: Object.freeze({
    tiers: Object.freeze([
      Object.freeze({ level: 1, slots: 1 }),
      Object.freeze({ level: 2, slots: 2 }),
      Object.freeze({ level: 11, slots: 3 }),
      Object.freeze({ level: 17, slots: 4 }),
    ]),
    slotLevelCap: 5,
  }),
  spellbook: Object.freeze({ baseKnown: 6, knownPerLevel: 2 }),
});

export const SYNTHETIC_2024_RULESET = deepFreeze({
  rulesetId: 'synthetic-dnd-2024',
  id: 'synthetic-dnd-2024',
  edition: '2024',
  rulesetVersion: 2,
  constants: {
    ...COMMON_CONSTANTS,
    casterFractions: { half: 'up', third: 'down' },
    rest: { longRestHitDice: 'all' },
  },
  capabilities: { weaponMastery: true },
  builder: {
    abilityScoreAdvancement: {
      baseLevels: [4, 8, 12, 16, 19],
      budget: 2,
      perAbilityMax: 2,
      featCategories: ['general'],
      categoriesByLevel: { 19: ['epicBoon'] },
      categoryAbilityCaps: { epicBoon: 30 },
    },
    backgroundAbilityGrant: { budget: 3, perAbilityMax: 2 },
    speciesAbilityGrant: false,
  },
});

export const SYNTHETIC_2014_RULESET = deepFreeze({
  rulesetId: 'synthetic-dnd-2014',
  id: 'synthetic-dnd-2014',
  edition: '2014',
  rulesetVersion: 2,
  constants: {
    ...COMMON_CONSTANTS,
    casterFractions: { half: 'down', third: 'down' },
    rest: { longRestHitDice: 'half' },
  },
  capabilities: { weaponMastery: false },
  builder: {
    abilityScoreAdvancement: {
      baseLevels: [4, 8, 12, 16, 19],
      budget: 2,
      perAbilityMax: 2,
      featCategories: ['general'],
      categoriesByLevel: {},
      categoryAbilityCaps: {},
    },
    backgroundAbilityGrant: false,
    speciesAbilityGrant: { budget: 2, perAbilityMax: 2 },
  },
});

function deepFreeze(value) {
  if (!value || typeof value !== 'object' || Object.isFrozen(value)) return value;
  for (const child of Object.values(value)) deepFreeze(child);
  return Object.freeze(value);
}
