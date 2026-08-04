// ═══════════════════════════════════════════════════════════════
//  rules/ruleset.js — edition system constants.
//
//  DEFAULT_RULESET is the engine's explicit standalone context. Provider
//  rulesets are complete unless they declare `extends: "dnd-2024"`; another
//  edition can therefore never inherit 2024 constants merely by omission.
//  Authority order remains printed class progression > resolved ruleset.
// ═══════════════════════════════════════════════════════════════

export const DEFAULT_RULESET = deepFreeze({
  rulesetId: 'dnd-2024',
  id: 'dnd-2024',
  edition: '2024',
  rulesetVersion: 1,
  constants: {
    // AB-4: per-ability cap; a cap-raising grant (Epic Boon) lifts it, never
    // past the hard ceiling (the 2024 absolute maximum for any score).
    abilityCap: 20,
    abilityCapHard: 30,
    // EQ-3: a character can attune to at most 3 magic items at once (2024 PHB).
    attunementLimit: 3,
    // SP-5: copying a spell into a wizard's spellbook costs 50 gp per spell
    // level (2024 PHB "Expanding and Replacing the Book"; min level 1).
    scrollCopyGpPerLevel: 50,
    // D&D 2024 standard point buy — 27 points; each BASE score 8–15; the cost
    // per point rises past 13.
    pointBuy: { budget: 27, min: 8, max: 15, cost: { 8: 0, 9: 1, 10: 2, 11: 3, 12: 4, 13: 5, 14: 7, 15: 9 } },
    // AB-1/AB-2: an ASI feat distributes 2 points (+2 or +1/+1); a background
    // distributes 3 points (+2/+1 or +1/+1/+1), max +2 to any one ability.
    // baseLevels are the class-agnostic ASI opportunities (class progressions
    // may add more — Fighter/Rogue extras come from the printed table).
    asi: { budget: 2, perMax: 2, bgBudget: 3, bgPerMax: 2, baseLevels: [4, 8, 12, 16, 19] },
    // MC-2: standard multiclass spell-slot table indexed by combined CASTER
    // LEVEL; row[i] = number of (i+1)-th-level slots.
    multiclassSlots: {
      1: [2], 2: [3], 3: [4, 2], 4: [4, 3], 5: [4, 3, 2], 6: [4, 3, 3], 7: [4, 3, 3, 1],
      8: [4, 3, 3, 2], 9: [4, 3, 3, 3, 1], 10: [4, 3, 3, 3, 2], 11: [4, 3, 3, 3, 2, 1],
      12: [4, 3, 3, 3, 2, 1], 13: [4, 3, 3, 3, 2, 1, 1], 14: [4, 3, 3, 3, 2, 1, 1],
      15: [4, 3, 3, 3, 2, 1, 1, 1], 16: [4, 3, 3, 3, 2, 1, 1, 1], 17: [4, 3, 3, 3, 2, 1, 1, 1, 1],
      18: [4, 3, 3, 3, 3, 1, 1, 1, 1], 19: [4, 3, 3, 3, 3, 2, 1, 1, 1], 20: [4, 3, 3, 3, 3, 2, 2, 1, 1],
    },
    // MC-2 rounding: how a class's levels count toward the combined caster
    // level. 2024 half-casters round UP (they cast from level 1 now) — a 2014
    // ruleset flips half to 'down'. Thirds round down in both editions.
    casterFractions: { half: 'up', third: 'down' },
    // Warlock Pact Magic: slots by ascending level tiers, ALL at one slot
    // level = min(slotLevelCap, ceil(level/2)), short-rest recharge.
    pactMagic: {
      tiers: [{ level: 1, slots: 1 }, { level: 2, slots: 2 }, { level: 11, slots: 3 }, { level: 17, slots: 4 }],
      slotLevelCap: 5,
    },
    // SP-5: wizard free-learn allotment — 6 spells at L1, +2 per wizard level
    // after (the 2024 rule; identical arithmetic to the old 2·L+4).
    spellbook: { baseKnown: 6, knownPerLevel: 2 },
    // 2024 Long Rest: "You regain all lost Hit Points and all spent Hit Point
    // Dice" — ALL of them. A 2014 ruleset sets 'half' (regain up to half your
    // total, the pre-2024 rule).
    rest: { longRestHitDice: 'all' },
  },
  capabilities: {
    // 2024-only subsystems + grant models — gate engine paths and Builder UI.
    weaponMastery: true,
    epicBoons: { atLevel: 19, abilityCap: 30 },
    backgroundAsi: true,   // 2024: the background grants the ASI …
    speciesAsi: false,     // … and species grant none (2014 flips both).
    originFeats: true,
  },
});

/** Resolve an explicit ruleset. Only declared compatible inheritance may use
 * the native 2024 base; otherwise the provider record is authoritative. */
export function resolveRuleset(record) {
  if (!isObj(record)) return DEFAULT_RULESET;
  if (record.extends === 'dnd-2024') return deepFreeze(merge(DEFAULT_RULESET, record));
  return record;
}

function isObj(v) { return !!v && typeof v === 'object' && !Array.isArray(v); }

function merge(def, part) {
  if (part === undefined) return def;
  if (isObj(def) && isObj(part)) {
    const out = {};
    for (const k of new Set([...Object.keys(def), ...Object.keys(part)])) out[k] = merge(def[k], part[k]);
    return out;
  }
  return part;
}

function deepFreeze(o) {
  for (const v of Object.values(o)) if (v && typeof v === 'object') deepFreeze(v);
  return Object.freeze(o);
}

