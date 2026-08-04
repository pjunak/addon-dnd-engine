// Synthetic, redistributable rules-data used by contract and engine tests.
// It mirrors the public record shapes without copying production book content.
import { DEFAULT_RULESET } from '../rules/ruleset.js';

export function makeFake() {
  const store = {
    class: {
      wizard: {
        id: 'wizard', name: 'Wizard', kind: 'class', hitDie: 'd6', savingThrows: ['INT', 'WIS'],
        spellcasting: { ability: 'INT', type: 'full', prepares: 'spellbook', ritual: true },
        weaponMastery: { count: 2 }, acFormulas: [],
        // Authoritative printed full-caster spell-slot progression (2024 PHB).
        // The L2 row is DELIBERATELY drifted: it name-drops the L18 'Spell
        // Mastery' (the real bug a stale book shipped). The engine must grant by
        // the feature RECORDS (below) and ignore the drifted string.
        progression: [
          { level: 1, cantripsKnown: 3, preparedSpells: 4, spellSlots: [2], features: ['Arcane Recovery'] },
          { level: 2, cantripsKnown: 3, preparedSpells: 5, spellSlots: [3], features: ['Scholar', 'Spell Mastery'] },
          { level: 5, cantripsKnown: 4, preparedSpells: 9, spellSlots: [4, 3, 2] },
          { level: 11, cantripsKnown: 5, preparedSpells: 15, spellSlots: [4, 3, 3, 2, 1, 1] },
          { level: 17, cantripsKnown: 5, preparedSpells: 21, spellSlots: [4, 3, 3, 3, 2, 1, 1, 1, 1] },
          { level: 19, cantripsKnown: 5, preparedSpells: 23, spellSlots: [4, 3, 3, 3, 3, 1, 1, 1, 1] },
        ],
      },
      barbarian: {
        id: 'barbarian', name: 'Barbarian', kind: 'class', hitDie: 'd12', savingThrows: ['STR', 'CON'],
        spellcasting: null, weaponMastery: { count: 2 },
        acFormulas: [{ id: 'ud', base: 10, addAbilities: ['DEX', 'CON'], requires: { noArmor: true } }],
        // FE-2/FE-3 resources exercising all three max shapes: progression table,
        // per-level multiple, ability modifier.
        classResources: [
          { key: 'rage', name: 'Rage', recharge: 'long', progression: [{ level: 1, max: 2 }, { level: 3, max: 3 }, { level: 6, max: 4 }] },
          { key: 'pool', name: 'Pool', recharge: 'short', perLevel: 5 },
          { key: 'insp', name: 'Insp', recharge: [{ on: 'short', amount: 1 }, { on: 'long', amount: 'full' }], abilityMod: 'CHA', min: 1 },
        ],
      },
      fighter: {
        id: 'fighter', name: 'Fighter', kind: 'class', hitDie: 'd10', savingThrows: ['STR', 'CON'],
        spellcasting: null, weaponMastery: { count: 3 }, acFormulas: [],
        startingProficiencies: { weapons: ['simple', 'martial'] },
      },
      paladin: {
        id: 'paladin', name: 'Paladin', kind: 'class', hitDie: 'd10', savingThrows: ['WIS', 'CHA'],
        spellcasting: { ability: 'CHA', type: 'half', prepares: 'list' }, weaponMastery: { count: 2 }, acFormulas: [],
        // Authoritative printed half-caster spell-slot progression (2024 PHB).
        progression: [
          { level: 1, preparedSpells: 2, spellSlots: [2] },
          { level: 5, preparedSpells: 6, spellSlots: [4, 2] },
          { level: 10, preparedSpells: 8, spellSlots: [4, 3, 2] },
          { level: 11, preparedSpells: 9, spellSlots: [4, 3, 3] },
          { level: 17, preparedSpells: 14, spellSlots: [4, 3, 3, 1] },
          { level: 19, preparedSpells: 15, spellSlots: [4, 3, 3, 1] },
        ],
      },
      // Ranger deliberately ships NO spellSlots → exercises the heuristic fallback.
      ranger: {
        id: 'ranger', name: 'Ranger', kind: 'class', hitDie: 'd10', savingThrows: ['STR', 'DEX'],
        spellcasting: { ability: 'WIS', type: 'half', prepares: 'list' }, weaponMastery: { count: 2 }, acFormulas: [],
        progression: [{ level: 1, preparedSpells: 2 }, { level: 5, preparedSpells: 6 }, { level: 10, preparedSpells: 8 }],
      },
      // Rogue-ish class: the 2024 'martial-finesse-or-light' weapon subset (PR-5)
      // — proficient with simple weapons + martial weapons that have Finesse or Light.
      rogue: {
        id: 'rogue', name: 'Rogue', kind: 'class', hitDie: 'd8', savingThrows: ['DEX', 'INT'],
        spellcasting: null, weaponMastery: { count: 2 }, acFormulas: [],
        // Skills choose + an L1 expertise choice — mirrors the real 2024 rogue
        // (exercises expertise materialization into the standalone fallback).
        startingProficiencies: { weapons: ['martial-finesse-or-light'], skills: { choose: 4, from: ['acrobatics', 'athletics', 'deception', 'insight', 'intimidation', 'investigation', 'perception', 'persuasion', 'sleightOfHand', 'stealth'] } },
        grants: { choices: [{ id: 'rogue-expertise-1', source: 'level:1', type: 'expertise', count: 2, prompt: 'Expertise (choose 2 skills)' }] },
        // Mirrors the real table's tricky rows: 'Expertise' repeats at two levels
        // (one record per occurrence — BOTH grant), and L3 mixes a recordless
        // generic label ('Rogue Subclass') with a real feature record.
        progression: [
          { level: 1, features: ['Expertise'] },
          { level: 3, features: ['Rogue Subclass', 'Steady Aim'] },
          { level: 6, features: ['Expertise'] },
        ],
      },
      sorcerer: {
        id: 'sorcerer', name: 'Sorcerer', kind: 'class', hitDie: 'd6', savingThrows: ['CON', 'CHA'],
        spellcasting: { ability: 'CHA', type: 'full', prepares: 'list' }, weaponMastery: { count: 0 }, acFormulas: [],
        progression: [{ level: 2, features: ['Metamagic'] }],
      },
      warlock: {
        id: 'warlock', name: 'Warlock', kind: 'class', hitDie: 'd8', savingThrows: ['WIS', 'CHA'],
        spellcasting: { ability: 'CHA', type: 'pact', prepares: 'list' }, weaponMastery: { count: 0 }, acFormulas: [],
        // 'Pact Magic' has NO feature record → the recordless-book string
        // recordless-provider fallback must grant it verbatim.
        progression: [{ level: 1, cantripsKnown: 2, preparedSpells: 2, features: ['Pact Magic'] }, { level: 5, cantripsKnown: 3, preparedSpells: 6 }],
      },
    },
    weapon: {
      longsword: { id: 'longsword', name: 'Longsword', kind: 'weapon', category: 'martial', range: 'melee', damage: '1d8', damageType: 'slashing', properties: ['versatile'], versatileDamage: '1d10', mastery: 'Sap' },
      dagger: { id: 'dagger', name: 'Dagger', kind: 'weapon', category: 'simple', range: 'melee', damage: '1d4', damageType: 'piercing', properties: ['finesse', 'light', 'thrown'], mastery: 'Nick' },
      rapier: { id: 'rapier', name: 'Rapier', kind: 'weapon', category: 'martial', range: 'melee', damage: '1d8', damageType: 'piercing', properties: ['finesse'], mastery: 'Vex' },
      greatsword: { id: 'greatsword', name: 'Greatsword', kind: 'weapon', category: 'martial', range: 'melee', damage: '2d6', damageType: 'slashing', properties: ['heavy', 'two-handed'], mastery: 'Graze' },
    },
    subclass: {
      'eldritch-knight': {
        id: 'eldritch-knight', name: 'Eldritch Knight', kind: 'subclass', classId: 'fighter',
        spellcasting: { ability: 'INT', type: 'third', prepares: 'list', startLevel: 3 },
        features: [{ level: 3, id: 'war-bond', name: 'War Bond' }],
        progression: [{ level: 3, cantripsKnown: 2, preparedSpells: 3, spellSlots: [2] }],
        classResources: [{ key: 'ek-pool', name: 'EK Pool', recharge: [{ on: 'short', amount: 'full' }], progression: [{ level: 3, max: 2 }, { level: 7, max: 3 }] }],
      },
      'life-domain': {
        id: 'life-domain', name: 'Life Domain', kind: 'subclass', classId: 'cleric',
        spells: [{ level: 3, ids: ['bless'], alwaysPrepared: true }],
      },
    },
    feat: {
      'fey-touched': { id: 'fey-touched', name: 'Fey Touched', category: 'general', grants: { abilityScoreIncrease: { choose: 1, amount: 1, from: ['INT', 'WIS', 'CHA'] }, spells: [{ ids: ['misty-step'], alwaysPrepared: true, free: '1/long' }] } },
      tough: { id: 'tough', name: 'Tough', category: 'general', grants: { hpPerLevel: 2 } },
      // Single-option half-feat → the Builder auto-applies its +1 (smoke.mjs).
      'great-weapon-master': { id: 'great-weapon-master', name: 'Great Weapon Master', category: 'general', grants: { abilityScoreIncrease: { choose: 1, amount: 1, from: ['STR'] } } },
      // 2024 Epic Boons (category epicBoon — the L19 slot). Real shapes: the
      // ability increase is the standard grant; "to a maximum of 30" is prose
      // (the cap rides on the category). One multi-option `from`, one
      // single-option (exercises the auto-apply path), one 'ANY' token
      // (Boon of Skill — "one ability score of your choice").
      'boon-of-fate': { id: 'boon-of-fate', name: 'Boon of Fate', category: 'epicBoon', grants: { abilityScoreIncrease: { choose: 1, amount: 1, from: ['INT', 'WIS', 'CHA'] } } },
      'boon-of-fortitude': { id: 'boon-of-fortitude', name: 'Boon of Fortitude', category: 'epicBoon', grants: { abilityScoreIncrease: { choose: 1, amount: 1, from: ['CON'] } } },
      'boon-of-skill': { id: 'boon-of-skill', name: 'Boon of Skill', category: 'epicBoon', grants: { abilityScoreIncrease: { choose: 1, amount: 1, from: ['ANY'] } } },
      'magic-initiate': { id: 'magic-initiate', name: 'Magic Initiate', category: 'origin', grants: { spells: [
        { id: 'mi-cantrips', choose: 2, spellLevel: 0, from: { class: ['wizard'] }, alwaysPrepared: true },
        { id: 'mi-spell', choose: 1, spellLevel: 1, from: { class: ['wizard'] }, alwaysPrepared: true, free: '1/long' },
      ] } },
    },
    spell: {
      bless: { id: 'bless', name: 'Bless', level: 1, school: 'Enchantment' },
      'misty-step': { id: 'misty-step', name: 'Misty Step', level: 2, school: 'Conjuration' },
      'dancing-lights': { id: 'dancing-lights', name: 'Dancing Lights', level: 0, school: 'Illusion' },
      'faerie-fire': { id: 'faerie-fire', name: 'Faerie Fire', level: 1, school: 'Evocation' },
      darkness: { id: 'darkness', name: 'Darkness', level: 2, school: 'Evocation' },
      druidcraft: { id: 'druidcraft', name: 'Druidcraft', level: 0, school: 'Transmutation' },
      // Wizard-tagged spells for the sheet's pickers/pools (smoke.mjs).
      'fire-bolt': { id: 'fire-bolt', name: 'Fire Bolt', level: 0, school: 'Evocation', classes: ['wizard'] },
      'mage-armor': { id: 'mage-armor', name: 'Mage Armor', level: 1, school: 'Abjuration', classes: ['wizard'] },
      fireball: { id: 'fireball', name: 'Fireball', level: 3, school: 'Evocation', classes: ['wizard'] },
      'detect-magic': { id: 'detect-magic', name: 'Detect Magic', level: 1, school: 'Divination', classes: ['wizard'], ritual: true },
    },
    armor: {
      breastplate: { id: 'breastplate', name: 'Breastplate', kind: 'armor', armorType: 'medium', baseAC: 14, dexCap: 2, acBonus: 0 },
      // A malformed body-armor record (garbage negative baseAC) — must never drag
      // AC below the 10+DEX unarmored floor.
      brokenplate: { id: 'brokenplate', name: 'Broken Plate', kind: 'armor', armorType: 'heavy', baseAC: -5, dexCap: 0, acBonus: 0 },
      leather: { id: 'leather', name: 'Leather Armor', kind: 'armor', armorType: 'light', baseAC: 11, dexCap: null, acBonus: 0 },
    },
    species: {
      dwarf: { id: 'dwarf', name: 'Dwarf', kind: 'species', speeds: { walk: 30 }, senses: { darkvision: 120 }, resistances: ['poison'], grants: { hpPerLevel: 1 }, lineages: [{ id: 'hill-dwarf', name: 'Hill Dwarf', grants: { hpPerLevel: 1 } }] },
      elf: { id: 'elf', name: 'Elf', kind: 'species', speeds: { walk: 30 }, senses: { darkvision: 60 }, resistances: [], lineages: [
        { id: 'drow', name: 'Drow', grants: { senses: { darkvision: 120 }, spells: [{ level: 0, ids: ['dancing-lights'], alwaysPrepared: true }, { level: 3, ids: ['faerie-fire'], alwaysPrepared: true, free: '1/long' }, { level: 5, ids: ['darkness'], alwaysPrepared: true, free: '1/long' }] } },
        { id: 'wood-elf', name: 'Wood Elf', grants: { speedBonus: 5, spells: [{ level: 0, ids: ['druidcraft'], alwaysPrepared: true }] } },
      ] },
    },
    background: {
      sage: { id: 'sage', name: 'Sage', kind: 'background', skillProficiencies: ['arcana', 'history'] },
      // Origin feat carrier → exercises the bg→feat→choose-grant chain (smoke.mjs).
      acolyte: { id: 'acolyte', name: 'Acolyte', kind: 'background', skillProficiencies: ['insight', 'religion'], originFeat: 'magic-initiate' },
    },
    // Feature records incl. an option-pool parent (Metamagic) + its category-tagged
    // options — exercises collectChoices' feature-grants + fromCategory expansion.
    // The wizard/rogue records back the collected-features step: records are the
    // grant identity (the wizard table's drifted 'Spell Mastery' string must NOT
    // grant); the two same-name rogue Expertise records must BOTH grant.
    feature: {
      'sorcerer-metamagic': { id: 'sorcerer-metamagic', name: 'Metamagic', kind: 'feature', classId: 'sorcerer', level: 2, localId: 'metamagic',
        grants: { choices: [{ id: 'metamagic', source: 'sorcerer:2', type: 'metamagic', count: 2, countByLevel: { '2': 2, '10': 4, '17': 6 }, fromCategory: 'metamagic', swappableOn: 'levelup' }] } },
      'metamagic-quickened-spell': { id: 'metamagic-quickened-spell', name: 'Quickened Spell', kind: 'feature', classId: 'sorcerer', level: 2, localId: 'quickened-spell', category: 'metamagic', prerequisite: { cost: '2 Sorcery Points' } },
      'metamagic-twinned-spell': { id: 'metamagic-twinned-spell', name: 'Twinned Spell', kind: 'feature', classId: 'sorcerer', level: 2, localId: 'twinned-spell', category: 'metamagic', prerequisite: { cost: '1 Sorcery Point' } },
      'wizard-arcane-recovery': { id: 'wizard-arcane-recovery', name: 'Arcane Recovery', kind: 'feature', classId: 'wizard', level: 1, localId: 'arcane-recovery' },
      'wizard-scholar': { id: 'wizard-scholar', name: 'Scholar', kind: 'feature', classId: 'wizard', level: 2, localId: 'scholar' },
      'wizard-spell-mastery': { id: 'wizard-spell-mastery', name: 'Spell Mastery', kind: 'feature', classId: 'wizard', level: 18, localId: 'spell-mastery' },
      'rogue-expertise': { id: 'rogue-expertise', name: 'Expertise', kind: 'feature', classId: 'rogue', level: 1, localId: 'expertise' },
      'rogue-steady-aim': { id: 'rogue-steady-aim', name: 'Steady Aim', kind: 'feature', classId: 'rogue', level: 3, localId: 'steady-aim' },
      'rogue-expertise-6': { id: 'rogue-expertise-6', name: 'Expertise', kind: 'feature', classId: 'rogue', level: 6, localId: 'expertise-6' },
    },
  };
  const byName = (kind, name) => Object.values(store[kind] || {}).find((r) => (r.name || '').toLowerCase() === String(name).toLowerCase()) || null;
  const vals = (kind) => Object.values(store[kind] || {});
  return {
    apiVersion: 1,
    listClasses: () => vals('class').map((c) => ({ id: c.id, name: c.name })),
    listSubclasses: (classId) => vals('subclass').filter((s) => !classId || s.classId === classId).map((c) => ({ id: c.id, name: c.name, classId: c.classId })),
    listFeatures: (q) => vals('feature').filter((f) =>
      (!q || !q.classId || f.classId === q.classId) &&
      (!q || !q.subclassId || f.subclassId === q.subclassId) &&
      (!q || !q.category || f.category === q.category) &&
      (!q || q.level == null || f.level === q.level))
      .map((f) => ({ id: f.id, name: f.name, kind: f.kind, classId: f.classId, subclassId: f.subclassId, level: f.level, category: f.category, grants: f.grants })),
    getFeature: (id) => (store.feature && store.feature[id]) || null,
    listSpecies: () => vals('species').map((s) => ({ id: s.id, name: s.name })),
    listBackgrounds: () => vals('background').map((b) => ({ id: b.id, name: b.name })),
    listFeats: (opts) => vals('feat').filter((f) => !opts || !opts.category || f.category === opts.category).map((f) => ({ id: f.id, name: f.name })),
    listSpells: (q) => vals('spell').filter((sp) =>
      (!q || q.level == null || sp.level === q.level) &&
      (!q || !q.class || (Array.isArray(sp.classes) && sp.classes.includes(q.class)))),
    listSkills: () => [], listArmor: () => vals('armor').map((a) => ({ id: a.id, name: a.name })),
    listWeapons: () => vals('weapon').map((w) => ({ id: w.id, name: w.name })),
    listEquipment: () => [...vals('armor'), ...vals('weapon')],
    getItem: (kind, id) => (store[kind] && store[kind][id]) || null,
    getItemByName: byName,
    getRecords: (kind) => vals(kind),
    getRuleset: () => DEFAULT_RULESET,
  };
}

