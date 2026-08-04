// ═══════════════════════════════════════════════════════════════
//  rules/engine.js — the pure, ruleset-parameterized D&D derivation engine.
//
//  No host, no DOM: every function takes plain decisions + a book-data accessor
//  `api`, so it is unit-testable with synthetic data. rules/api.js binds it to
//  the currently selected `dnd5e.rules-data` service.
//
//  It encodes the SYSTEM (how PB / mods / HP / AC / saves / slots are computed)
//  and reads CONTENT (class/species/armor records) from `api` — so new content
//  is a provider data change and never touches this file. Each pipeline step
//  is error-isolated: a throw becomes a warning and the sheet is still returned
//  through the public service boundary.
// ═══════════════════════════════════════════════════════════════

import { DEFAULT_RULESET, resolveRuleset } from './ruleset.js';
import {
  activationKey,
  applyChoicePackages,
  collectGrantSources,
  selectedFeatRecords,
} from './grants.js';
export { DEFAULT_RULESET, resolveRuleset };

export const ABILITIES = ['STR', 'DEX', 'CON', 'INT', 'WIS', 'CHA'];

// Skill → governing ability. SYSTEM knowledge (not content), so it lives here
// and the engine never needs the compendium just to total a skill.
export const SKILL_ABILITY = {
  acrobatics: 'DEX', animalHandling: 'WIS', arcana: 'INT', athletics: 'STR',
  deception: 'CHA', history: 'INT', insight: 'WIS', intimidation: 'CHA',
  investigation: 'INT', medicine: 'WIS', nature: 'INT', perception: 'WIS',
  performance: 'CHA', persuasion: 'CHA', religion: 'INT', sleightOfHand: 'DEX',
  stealth: 'DEX', survival: 'WIS',
};

export const num = (v, d = 0) => { const n = Number(v); return Number.isFinite(n) ? n : d; };
export const abilityMod = (score) => Math.floor((num(score, 10) - 10) / 2);
export const proficiencyBonus = (totalLevel) => 2 + Math.floor((Math.max(1, num(totalLevel, 1)) - 1) / 4);
export const dieSize = (hitDie) => num(String(hitDie || '').replace(/^d/i, ''), 8);
/** Average of a hit die rounded UP (d6→4, d8→5, d10→6, d12→7) — the per-level
 *  HP value and the hit-die-spend heal. SYSTEM rule, one site (was duplicated
 *  as DIE_AVG tables in entry.js + panel.sheet.js). */
export const hitDieAvg = (hitDie) => Math.floor(dieSize(hitDie) / 2) + 1;

// ── System constants ────────────────────────────────────────────────
// The numbers below live in rules/ruleset.js as DEFAULT_RULESET (the 2024
// values) and may be overridden per edition by the data addon's `ruleset`
// record. Every helper takes an optional resolved-ruleset `rs` that defaults
// to the 2024 defaults, so existing call sites and standalone use never break.
// The named exports (ASI_RULES, POINT_BUY, …) stay as views of the DEFAULTS
// for back-compat; ruleset-aware callers read `rulesApi.getRuleset()` instead.

export const scrollCopyCost = (level, rs = DEFAULT_RULESET) =>
  num(rs.constants.scrollCopyGpPerLevel, 50) * Math.max(1, num(level, 1));

// 2024 ASI budgets (AB-1/AB-2) — SYSTEM rules the Builder's pickers consume.
export const ASI_RULES = DEFAULT_RULESET.constants.asi;

// Ability score caps (AB-4): 20 by default; a cap-raising grant (2024 Epic
// Boon: "increase … by 1, to a maximum of 30") lifts it, never past the hard
// ceiling.
export const ABILITY_CAP = DEFAULT_RULESET.constants.abilityCap;
export const ABILITY_CAP_HARD = DEFAULT_RULESET.constants.abilityCapHard;
/** Eligible abilities of a feat's abilityScoreIncrease grant. The 2024 data's
 *  'ANY' token ("one ability score of your choice" — e.g. Boon of Skill) means
 *  all six. [] when the grant is absent/malformed. */
export const featAsiFrom = (asi) => (asi && Array.isArray(asi.from) ? (asi.from.includes('ANY') ? ABILITIES.slice() : asi.from) : []);
/** The raised per-ability cap a feat's ASI carries, or null when the default
 *  applies. 2024 Epic Boons print "to a maximum of 30" as prose, so the cap
 *  rides on the CATEGORY (gated by the ruleset's epicBoons capability). */
export const featAbilityCap = (feat, rs = DEFAULT_RULESET) => {
  const boons = rs.capabilities.epicBoons;
  return (boons && feat && feat.category === 'epicBoon') ? num(boons.abilityCap, rs.constants.abilityCapHard) : null;
};

// Standard point buy. `pointCost` clamps out-of-range scores into [min,max]
// for costing; `pointsSpent` totals a {STR..CHA} base map.
export const POINT_BUY = DEFAULT_RULESET.constants.pointBuy;
export const pointCost = (v, rs = DEFAULT_RULESET) => {
  const pb = rs.constants.pointBuy;
  return pb.cost[Math.max(pb.min, Math.min(pb.max, num(v, pb.min)))] || 0;
};
export const pointsSpent = (base, rs = DEFAULT_RULESET) => ABILITIES.reduce((sum, a) => sum + pointCost(base && base[a], rs), 0);

/** HP clamp — one rule for all sites. With a max>0, clamp into [0, max];
 *  with no max set (0), only floor at 0 (the ± action stays usable). */
export const clampHp = (hp, maxHp) => {
  const h = num(hp, 0), m = num(maxHp, 0);
  return m > 0 ? Math.max(0, Math.min(m, h)) : Math.max(0, h);
};

// Standard multiclass spell-slot table, indexed by combined CASTER LEVEL
// (MC-2). slots[i] = number of (i+1)-th-level slots. Single full casters land on
// their own table row; this also covers multiclass + half/third casters.
// A caster level above the table's top row clamps to the top row.
export const multiclassSlots = (casterLevel, rs = DEFAULT_RULESET) => {
  const table = rs.constants.multiclassSlots || {};
  let lvl = Math.max(0, Math.floor(num(casterLevel, 0)));
  const top = Object.keys(table).reduce((m, k) => Math.max(m, num(k)), 0);
  if (lvl > top) lvl = top;
  const row = table[lvl];
  return Array.isArray(row) ? row.slice() : [];
};

/** A class's contribution to the combined caster level (MC-2): full = level;
 *  half/third divide by 2/3 with the ruleset's rounding direction. 2024 PHB
 *  Multiclassing, Spell Slots: "Half your levels (round up) in the Paladin and
 *  Ranger classes; one third of your Fighter or Rogue levels (round down) if
 *  you have the Eldritch Knight or Arcane Trickster subclass." The half-caster
 *  round-UP is a 2024 change from 2014 (they cast from level 1 now) — a 2014
 *  ruleset record ships `casterFractions.half: 'down'`. */
export function casterContribution(type, level, rs = DEFAULT_RULESET) {
  if (type === 'full') return level;
  const div = type === 'half' ? 2 : type === 'third' ? 3 : 0;
  if (!div) return 0;   // 'pact' contributes 0 — Warlock Pact Magic never combines (see pactMagic)
  const dir = (rs.constants.casterFractions || {})[type];
  return dir === 'up' ? Math.ceil(level / div) : Math.floor(level / div);
}

/** Warlock Pact Magic: a small pool of slots, ALL at one level, recharged on a
 *  SHORT rest — distinct from Spellcasting slots and NOT combined in multiclass.
 *  2024: 1 slot (L1), 2 (L2–10), 3 (L11–16), 4 (L17+); slot level:
 *  min(slotLevelCap, ⌈level/2⌉). Derived by level from the ruleset's ascending
 *  tiers (the class progression carries no pact table). null below tier 1. */
export function pactMagic(level, rs = DEFAULT_RULESET) {
  const L = Math.max(0, Math.floor(num(level, 0)));
  if (L < 1) return null;
  const pm = rs.constants.pactMagic || {};
  let slots = 0;
  for (const t of pm.tiers || []) if (L >= num(t.level)) slots = num(t.slots);
  if (slots < 1) return null;
  return { slots, level: Math.min(num(pm.slotLevelCap, 5), Math.ceil(L / 2)) };
}

/** Pick the progression row at `level` (or the highest row ≤ level — handles the
 *  abbreviated seed tables that stop at ~level 5). */
function progressionAt(progression, level) {
  if (!Array.isArray(progression) || !progression.length) return null;
  let best = null;
  for (const row of progression) if (num(row.level) <= level && (!best || row.level > best.level)) best = row;
  return best || progression[0];
}

/** Normalize decisions into an ordered class list with resolved records.
 *  Accepts the rich `classes:[{classId,level,subclass}]` shape OR the current
 *  flat `{className, subclass, level}` sheet shape (MC-1 migration not done yet). */
export function resolveClasses(cd, api, warn) {
  const out = [];
  const lookup = (idOrName) => {
    if (!api) return null;
    return (api.getItem && api.getItem('class', idOrName)) || (api.getItemByName && api.getItemByName('class', idOrName)) || null;
  };
  if (Array.isArray(cd.classes) && cd.classes.length) {
    for (const c of cd.classes) {
      // A PLACEHOLDER roster row (the Builder's "＋ Add class" before a class is
      // picked) contributes nothing — counting it would inflate total level, PB
      // and HP (an empty row used to add a phantom avg-d8 level). A row with a
      // A class id the provider cannot resolve still counts as free text.
      if (!c || !c.classId) continue;
      const rec = lookup(c.classId);
      if (!rec && api) warn('Unknown class: ' + c.classId);
      out.push({ classId: c.classId, name: rec ? rec.name : c.classId, level: Math.max(1, num(c.level, 1)), subclass: c.subclass || '', record: rec });
    }
  } else if (cd.className) {
    const rec = lookup(cd.className);
    if (!rec && api) warn('Unknown class: ' + cd.className);
    out.push({ classId: rec ? rec.id : cd.className, name: rec ? rec.name : cd.className, level: Math.max(1, num(cd.level, 1)), subclass: cd.subclass || '', record: rec });
  }
  return out;
}

// ── derived computations (each pure, given resolved inputs) ─────────

/** HP-1: the character's first level gets the MAX of its hit die; every other
 *  level = average (round up) of that level's class die; + CON×totalLevel +
 *  per-level species bonuses (e.g. Dwarven Toughness, HP-3). The max die is
 *  awarded to the first level iterated — for the common single-class case that is
 *  simply the class's die; multiclass entry order beyond the first doesn't change
 *  the total, so no "origin class" is tracked. */
export function computeMaxHp(classes, conMod, hpPerLevel) {
  let hp = 0, charLevel = 0, maxDieAwarded = false;
  for (const c of classes) {
    const die = dieSize(c.record && c.record.hitDie);
    for (let i = 0; i < c.level; i++) {
      charLevel++;
      // Max hit die for the very first character level; average thereafter.
      if (!maxDieAwarded) { hp += die; maxDieAwarded = true; }
      else hp += Math.floor(die / 2) + 1;
    }
  }
  if (!charLevel) return 0;
  return hp + conMod * charLevel + num(hpPerLevel) * charLevel;
}

/** AC-1/AC-2/AC-3: collect every eligible base-AC formula (equipped armor with
 *  its dex cap, else each class Unarmored-Defense formula, else 10+DEX), take the
 *  BEST, then add a shield + any flat bonuses. Never stacks two bases. */
export function computeArmorClass(cd, mods, classes, api, extraFormulas = [], abilities = {}) {
  const dex = mods.DEX;
  const inv = Array.isArray(cd.inventory) ? cd.inventory : [];
  const armorRec = (it) => {
    if (!api) return null;
    return (it.ref && api.getItem && api.getItem('armor', it.ref)) || (it.name && api.getItemByName && api.getItemByName('armor', it.name)) || null;
  };
  let bodyArmor = null, shield = null;
  for (const it of inv) {
    if ((it.location || '') !== 'equipped') continue;
    const rec = armorRec(it);
    if (!rec) continue;
    if (rec.armorType === 'shield') shield = shield || rec;
    else if (['light', 'medium', 'heavy'].includes(rec.armorType)) bodyArmor = bodyArmor || rec;
  }
  const candidates = [];
  if (bodyArmor) {
    const dexPart = bodyArmor.dexCap === 0 ? 0 : bodyArmor.dexCap == null ? dex : Math.min(dex, num(bodyArmor.dexCap));
    candidates.push({ id: 'armor:' + bodyArmor.id, label: bodyArmor.name, value: num(bodyArmor.baseAC, 10) + dexPart });
  } else {
    const formulas = classes.flatMap((c) => (c.record && c.record.acFormulas) || [])
      .concat(extraFormulas || []);
    for (const f of formulas) {
      if (f.requires && f.requires.noShield && shield) continue;
      const add = (f.addAbilities || []).reduce((s, ab) => s + num(mods[ab]), 0);
      candidates.push({ id: f.id, label: f.name || f.id, value: num(f.base, 10) + add });
    }
  }
  // ALWAYS offer the unarmored 10+DEX candidate so the reducer floors there: a
  // malformed body-armor record (e.g. a negative/garbage baseAC) can never drop
  // AC below the bare-minimum 10+DEX every creature has.
  candidates.push({ id: 'unarmored', label: 'Unarmored', value: 10 + dex });
  const best = candidates.reduce((a, b) => (b.value > a.value ? b : a), { value: -Infinity, label: '', id: '' });
  const shieldBonus = shield ? num(shield.acBonus, 2) : 0;
  const strengthRequirement = bodyArmor
    ? num(String(bodyArmor.strReq || '').match(/\d+/)?.[0], num(bodyArmor.strReq))
    : 0;
  const strengthScore = num(abilities.STR && abilities.STR.score, 10 + num(mods.STR) * 2);
  const restrictions = bodyArmor ? {
    armorId: bodyArmor.id,
    strengthRequirement,
    meetsStrength: !strengthRequirement || strengthScore >= strengthRequirement,
    speedPenalty: strengthRequirement && strengthScore < strengthRequirement ? 10 : 0,
    stealthDisadvantage: !!bodyArmor.stealthDisadvantage,
  } : null;
  return {
    value: best.value + shieldBonus,
    base: best.label,
    shield: shieldBonus,
    candidates,
    restrictions,
  };
}

// EQ-5: which ability a weapon uses — finesse takes the better of STR/DEX,
// ranged uses DEX, everything else STR.
export function weaponAbilityMod(rec, mods, preferredAbility) {
  const str = num(mods.STR), dex = num(mods.DEX);
  const props = rec.properties || [];
  let value = props.includes('finesse') ? Math.max(str, dex)
    : rec.range === 'ranged' ? dex
    : str;
  if (preferredAbility) value = Math.max(value, num(mods[preferredAbility]));
  return value;
}

/** Union of class weapon proficiencies (PR-5). Tokens: 'simple', 'martial',
 *  'martial-light' (the 2024 Monk subset), and
 *  'martial-finesse-or-light' (the 2024 Rogue subset).
 *  Multiclassing grants only a REDUCED starting set: when a NON-origin class
 *  (index > 0) declares `multiclassProficiencies`, its `weapons` list is used
 *  INSTEAD of the full starting set (an absent/empty list grants nothing —
 *  declaring the field means "this is the complete multiclass grant").
 *  Records without the field keep today's behavior (full starting set), so
 *  books that don't ship it yet are unaffected. */
export function classWeaponProf(classes, extra = []) {
  const p = {
    simple: false,
    martial: false,
    martialLight: false,
    martialFinesseLight: false,
    ids: new Set(),
  };
  classes.forEach((c, i) => {
    const rec = c.record;
    if (!rec) return;
    const src = (i > 0 && rec.multiclassProficiencies)
      ? rec.multiclassProficiencies.weapons
      : rec.startingProficiencies && rec.startingProficiencies.weapons;
    for (const tok of src || []) {
      if (tok === 'simple') p.simple = true;
      else if (tok === 'martial') p.martial = true;
      else if (tok === 'martial-light') { p.simple = true; p.martialLight = true; }
      else if (tok === 'martial-finesse-or-light') { p.simple = true; p.martialFinesseLight = true; }
      else p.ids.add(tok);
    }
  });
  for (const tok of extra || []) {
    if (tok === 'simple') p.simple = true;
    else if (tok === 'martial') p.martial = true;
    else if (tok === 'martial-light') { p.simple = true; p.martialLight = true; }
    else if (tok === 'martial-finesse-or-light') { p.simple = true; p.martialFinesseLight = true; }
    else p.ids.add(tok);
  }
  return p;
}
function weaponProficient(rec, p) {
  if (p.ids && p.ids.has(rec.id)) return true;
  if (rec.category === 'simple') return !!p.simple;
  if (rec.category === 'martial') {
    if (p.martial) return true;
    if (p.martialLight && (rec.properties || []).includes('light')) return true;
    if (p.martialFinesseLight && ((rec.properties || []).includes('finesse') || (rec.properties || []).includes('light'))) return true;
  }
  return false;
}

/** EQ-5: attack bonus + damage for one weapon. attack = abilityMod +
 *  (proficient ? PB : 0); damage adds the ability modifier. Magic +N is 0 for
 *  now (no magic weapons in the seed). */
export function computeWeaponAttack(rec, mods, pb, profW, masterySet, preferredAbility) {
  const prof = weaponProficient(rec, profW);
  const abil = weaponAbilityMod(rec, mods, prof ? preferredAbility : null);
  const dmgSuffix = abil ? ' ' + (abil > 0 ? '+' : '') + abil : '';
  return {
    ref: rec.id, name: rec.name,
    attackBonus: abil + (prof ? pb : 0),
    damage: (rec.damage || '') + dmgSuffix,
    versatileDamage: rec.versatileDamage ? rec.versatileDamage + dmgSuffix : null,
    damageType: rec.damageType || '', properties: rec.properties || [],
    mastery: rec.mastery || '', masteryActive: !!(masterySet && masterySet.has(rec.id)),
    proficient: prof,
  };
}

// ── the pipeline ───────────────────────────────────────────────────

/**
 * Hydrate player DECISIONS into a computed sheet. NEVER throws — every step is
 * error-isolated and failures accumulate in `warnings`. Returns { sheet, warnings }.
 * The engine only proposes; the sheet layer lets a stored override win.
 * `ruleset` is the data provider's (possibly partial) `ruleset` record —
 * resolved per constant over the 2024 defaults, so omitting it keeps
 * every existing call site byte-identical.
 */
export function hydrate(decisions, api, ruleset) {
  const rs = resolveRuleset(ruleset);
  const cd = decisions || {};
  const warnings = [];
  const warn = (m) => { if (m) warnings.push(String(m)); };
  const step = (fn) => { try { fn(); } catch (e) { warn('engine: ' + (e && e.message || e)); } };

  const sheet = {
    abilities: {},
    derived: {},
    proficiencies: {
      saves: {},
      skills: {},
      armor: [],
      weapons: [],
      tools: [],
      languages: [],
    },
    features: [],
  };
  const mods = {};

  // Abilities (AB-1/AB-2/AB-4): final = base + Σ ability grants (background
  // ASI, half-feats, Epic Boons …), clamped to a PER-ABILITY cap: 20 by
  // default, raised by a grant carrying `cap` (2024 Epic Boons: "+1, to a
  // maximum of 30") — the raise applies only to abilities that grant touches,
  // with 30 as the hard ceiling (the 2024 absolute maximum). `baseStats` is
  // preferred; `abilities` is the back-compat fallback (the flat sheet stores
  // final scores directly). Each grant is { source, assign: { STR:+2, … },
  // cap? }. Shape: abilities[a] = {base, score, mod, bonus}.
  step(() => {
    const base = cd.baseStats || cd.abilities || {};
    const grants = Array.isArray(cd.abilityGrants) ? cd.abilityGrants : [];
    const capDefault = num(rs.constants.abilityCap, 20), capHard = num(rs.constants.abilityCapHard, 30);
    for (const a of ABILITIES) {
      let bonus = 0, cap = capDefault;
      for (const g of grants) {
        const v = g && g.assign && g.assign[a];
        if (!v) continue;
        bonus += num(v);
        if (num(g.cap) > cap) cap = Math.min(capHard, num(g.cap));
      }
      const score = Math.min(cap, num(base[a], 10) + bonus);
      const m = abilityMod(score);
      mods[a] = m;
      sheet.abilities[a] = { base: num(base[a], 10), score, mod: m, bonus };
    }
  });

  // Classes + total level + proficiency bonus (PB from TOTAL level — PR-6).
  let classes = [];
  let totalLevel = 1;
  step(() => {
    classes = resolveClasses(cd, api, warn);
    totalLevel = classes.length ? classes.reduce((s, c) => s + c.level, 0) : Math.max(1, num(cd.level, 1));
    sheet.classes = classes.map((c) => ({ classId: c.classId, name: c.name, level: c.level, subclass: c.subclass, hitDie: c.record && c.record.hitDie }));
    sheet.totalLevel = totalLevel;
    sheet.derived.proficiencyBonus = proficiencyBonus(totalLevel);
    if (classes[0] && classes[0].record) sheet.class = classes[0].record;     // back-compat
    sheet.derived.hitDie = (classes[0] && classes[0].record && classes[0].record.hitDie) || null;
  });
  const pb = sheet.derived.proficiencyBonus;

  // Species (speed, senses take-highest, resistances, per-level HP bonus — SB-3/SB-4).
  let species = null, hpPerLevel = 0, lineage = null;
  step(() => {
    if (cd.race || cd.species) {
      species = (api && (api.getItemByName && api.getItemByName('species', cd.race || cd.species))) || null;
      if (!species && api) warn('Unknown species: ' + (cd.race || cd.species));
    }
    sheet.species = species || undefined;
    lineage = species && cd.lineage ? (species.lineages || []).find((l) => l.id === cd.lineage) : null;
    let darkvision = 0, speedBonus = 0;
    const resistances = new Set();
    if (species) {
      if (species.senses && species.senses.darkvision) darkvision = Math.max(darkvision, num(species.senses.darkvision));
      for (const r of species.resistances || []) resistances.add(r);
      if (species.grants && species.grants.hpPerLevel) hpPerLevel += num(species.grants.hpPerLevel);
    }
    // Selected lineage grants (SB-3): darkvision take-highest, speed bonus,
    // resistances, per-level HP (Dwarven Toughness). Lineage SPELLS are granted
    // in the spellcasting step (level-gated + provenance-tagged).
    const lg = lineage && lineage.grants;
    if (lg) {
      if (lg.senses && lg.senses.darkvision) darkvision = Math.max(darkvision, num(lg.senses.darkvision));
      for (const r of lg.resistances || []) resistances.add(r);
      if (lg.hpPerLevel) hpPerLevel += num(lg.hpPerLevel);
      if (lg.speedBonus) speedBonus += num(lg.speedBonus);
    }
    sheet.speed = (species && species.speeds && species.speeds.walk ? num(species.speeds.walk) : 30) + speedBonus;
    sheet.derived.speed = sheet.speed;
    sheet.senses = darkvision ? { darkvision } : {};
    sheet.resistances = [...resistances];
  });

  // Background (skill proficiencies it grants — AB-1's ability split needs the
  // Builder's choice, so only the deterministic grants are applied here).
  let background = null;
  step(() => {
    if (cd.background) {
      background = (api && api.getItemByName && api.getItemByName('background', cd.background)) || null;
      if (!background && api) warn('Unknown background: ' + cd.background);
    }
  });

  const featRecords = selectedFeatRecords(cd, api);
  const grantSources = applyChoicePackages(collectGrantSources({
    classes,
    species,
    lineage,
    background,
    featRecords,
    api,
    totalLevel,
  }), cd.featureChoices);
  const grantValues = (field) => grantSources.flatMap(({ grants }) => []
    .concat(Array.isArray(grants && grants[field]) ? grants[field] : [])
    .concat(Array.isArray(grants && grants.proficiencies && grants.proficiencies[field])
      ? grants.proficiencies[field]
      : []));

  const activeModifiers = [];
  step(() => {
    const active = cd.activeFeatures || {};
    const activations = [];
    let wearingArmor = false;
    let usingShield = false;
    for (const item of Array.isArray(cd.inventory) ? cd.inventory : []) {
      if (item.location !== 'equipped' || !api) continue;
      const armor = (item.ref && api.getItem && api.getItem('armor', item.ref))
        || (item.name && api.getItemByName && api.getItemByName('armor', item.name));
      if (!armor) continue;
      if (armor.armorType === 'shield') usingShield = true;
      else if (['light', 'medium', 'heavy'].includes(armor.armorType)) wearingArmor = true;
    }
    for (const source of grantSources) {
      const declared = []
        .concat(Array.isArray(source.record.activations) ? source.record.activations : [])
        .concat(Array.isArray(source.grants.activations) ? source.grants.activations : []);
      for (const activation of declared) {
        if (!activation || !activation.id || num(activation.minLevel, 1) > source.level) continue;
        const key = activationKey(source, activation);
        const restrictions = activation.restrictions || {};
        const allowed = !(restrictions.noArmor && wearingArmor)
          && !(restrictions.noShield && usingShield);
        const enabled = !!active[key] && allowed;
        activations.push({
          key,
          id: activation.id,
          name: activation.name || activation.id,
          source: source.source,
          resource: activation.resource || null,
          exclusiveGroup: activation.exclusiveGroup || null,
          active: enabled,
          available: allowed,
        });
        if (enabled) activeModifiers.push(...(activation.modifiers || []));
      }
    }
    sheet.activations = activations;
  });

  // HP (HP-1/HP-2/HP-3) — per-level bonuses from species/lineage plus feats
  // (Tough = +2/level; the data carries grants.hpPerLevel).
  step(() => {
    let featHp = 0;
    for (const { record } of featRecords) {
      if (record.grants && record.grants.hpPerLevel) featHp += num(record.grants.hpPerLevel);
    }
    const conMod = mods.CON || 0;
    const perLevelMisc = hpPerLevel + featHp;   // species + lineage + feats, per level
    const maxHp = computeMaxHp(classes, conMod, perLevelMisc);
    // Breakdown for the sheet's HP legend (mirrors computeMaxHp's HP-1 rule): the
    // raw hit-dice subtotal (first level max die, others average), CON×level, and
    // the per-level misc bonuses. dice + conTotal + miscTotal === maxHp.
    let dice = 0, charLevel = 0, maxDieAwarded = false;
    for (const c of classes) {
      const die = dieSize(c.record && c.record.hitDie);
      for (let i = 0; i < c.level; i++) { charLevel++; if (!maxDieAwarded) { dice += die; maxDieAwarded = true; } else dice += Math.floor(die / 2) + 1; }
    }
    sheet.hp = { max: maxHp, breakdown: { dice, conMod, conTotal: conMod * charLevel, miscPerLevel: perLevelMisc, miscTotal: perLevelMisc * charLevel, level: charLevel } };
    sheet.derived.maxHp = maxHp;
  });

  // AC (AC-1).
  step(() => {
    const extraFormulas = grantSources
      .filter(({ source }) => source.type !== 'class')
      .flatMap(({ record, grants }) => []
        .concat(Array.isArray(record.acFormulas) ? record.acFormulas : [])
        .concat(Array.isArray(grants.acFormulas) ? grants.acFormulas : []));
    const ac = computeArmorClass(cd, mods, classes, api, extraFormulas, sheet.abilities);
    const speciesBonus = num(species && species.grants && species.grants.acBonus);
    const activeBonus = activeModifiers
      .filter((modifier) => modifier && modifier.target === 'armorClass')
      .reduce((sum, modifier) => sum
        + num(modifier.add)
        + num(mods[modifier.addAbility]), 0);
    if (speciesBonus || activeBonus) {
      ac.value += speciesBonus + activeBonus;
      if (speciesBonus) ac.speciesBonus = speciesBonus;
      if (activeBonus) ac.activeBonus = activeBonus;
    }
    sheet.ac = ac;
    sheet.armorRestrictions = ac.restrictions;
    if (ac.restrictions && ac.restrictions.speedPenalty) {
      sheet.speed = Math.max(0, sheet.speed - ac.restrictions.speedPenalty);
      sheet.derived.speed = sheet.speed;
    }
    sheet.derived.armorClass = ac.value;
  });

  // Initiative (CX-2): DEX + feat bonuses. A feat record carrying a structured
  // `modifiers: [{target:'initiative', add:'PB'|<number>}]` is the authority
  // (2024 Alert adds PB; a 2014 Alert record would add a flat 5) — the
  // hardcoded alert→PB check survives only as the fallback for books that
  // predate the field.
  step(() => {
    let init = num(mods.DEX);
    for (const f of Array.isArray(cd.feats) ? cd.feats : []) {
      const fid = f && (f.featId || f.id || f);
      const frec = fid && api && api.getItem ? api.getItem('feat', fid) : null;
      if (frec && Array.isArray(frec.modifiers)) {
        for (const m of frec.modifiers) if (m && m.target === 'initiative') init += m.add === 'PB' ? pb : num(m.add);
      } else if (fid === 'alert') {
        init += pb;
      }
    }
    sheet.derived.initiative = init;
  });

  // Saving throws (PR-4: proficiency only from the FIRST class; union with any
  // manual saveProf the sheet carries).
  step(() => {
    const firstSaves = (classes[0] && classes[0].record && classes[0].record.savingThrows) || [];
    const manual = cd.saveProf || {};
    const granted = new Set(grantValues('savingThrows').concat(cd.saveProficiencies || []));
    sheet.saves = {};
    for (const a of ABILITIES) {
      const proficient = firstSaves.includes(a) || granted.has(a) || !!manual[a];
      sheet.proficiencies.saves[a] = proficient;
      sheet.saves[a] = { mod: num(mods[a]), proficient, total: num(mods[a]) + (proficient ? pb : 0) };
    }
  });

  // Skills (PR-1/PR-2): proficiency = resolved choices (class skill picks, via
  // `skillProficiencies[]`) ∪ background grants; standalone falls back to the
  // manual `skillProf{}` map. Expertise from `skillExpertise{}` (the Builder's
  // expertise picks), and only counts where the character is proficient.
  step(() => {
    const manual = cd.skillProf || {};
    const resolved = Array.isArray(cd.skillProficiencies)
      ? cd.skillProficiencies
      : Object.keys(manual).filter((k) => manual[k]);
    const bgSkills = (background && background.skillProficiencies) || [];
    const speciesSkills = Array.isArray(cd.speciesSkillProficiencies)
      ? cd.speciesSkillProficiencies
      : [];
    const grantedSkills = new Set(grantValues('skills'));
    const expertise = {
      ...(cd.skillExpertise || {}),
      ...Object.fromEntries(grantValues('expertise').map((id) => [id, true])),
    };
    sheet.skills = {};
    for (const id of Object.keys(SKILL_ABILITY)) {
      const ab = SKILL_ABILITY[id];
      const proficient = resolved.includes(id) || bgSkills.includes(id)
        || speciesSkills.includes(id) || grantedSkills.has(id);
      const exp = !!expertise[id] && proficient;
      sheet.proficiencies.skills[id] = exp ? 'expertise' : proficient ? 'proficient' : 'none';
      const bonus = (exp ? 2 : proficient ? 1 : 0) * pb;
      sheet.skills[id] = { ability: ab, mod: num(mods[ab]), proficient, expertise: exp, total: num(mods[ab]) + bonus };
    }
    sheet.passives = { perception: 10 + sheet.skills.perception.total };
    sheet.derived.passivePerception = sheet.passives.perception;
  });

  // Armor and tool proficiencies use the origin class's full starting grants
  // and each later class's reduced multiclass grants. Background, species, and
  // resolved generic choices are additive.
  step(() => {
    const armor = new Set();
    const tools = new Set();
    const weapons = new Set(grantValues('weapons').concat(cd.weaponProficiencies || []));
    classes.forEach((c, index) => {
      const rec = c.record;
      if (!rec) return;
      const source = index > 0 && rec.multiclassProficiencies
        ? rec.multiclassProficiencies
        : rec.startingProficiencies;
      for (const id of (source && source.armor) || []) armor.add(id);
      for (const id of (source && source.tools) || []) tools.add(id);
    });
    if (background && background.toolProficiency && !background.toolProficiencyChoice) {
      const declared = background.toolProficiency;
      const tool = api && (
        (api.getItem && api.getItem('tool', declared))
        || (api.getItemByName && api.getItemByName('tool', declared))
      );
      if (tool && tool.id) tools.add(tool.id);
    }
    if (cd.backgroundToolProficiency) tools.add(cd.backgroundToolProficiency);
    for (const id of Array.isArray(cd.toolProficiencies) ? cd.toolProficiencies : []) tools.add(id);
    for (const id of Array.isArray(cd.speciesToolProficiencies) ? cd.speciesToolProficiencies : []) tools.add(id);
    for (const id of grantValues('armor').concat(cd.armorProficiencies || [])) armor.add(id);
    for (const id of grantValues('tools')) tools.add(id);
    sheet.proficiencies.armor = [...armor];
    sheet.proficiencies.weapons = [...weapons];
    sheet.proficiencies.tools = [...tools];
  });

  step(() => {
    const languages = new Set(grantValues('languages').concat(cd.languageProficiencies || []));
    const resistances = new Set((sheet.resistances || [])
      .concat(grantValues('resistances'))
      .concat(cd.damageResistances || []));
    const conditionImmunities = new Set(grantValues('conditionImmunities')
      .concat(cd.conditionImmunities || []));
    const damageImmunities = new Set(grantValues('damageImmunities'));
    for (const modifier of activeModifiers) {
      if (!modifier) continue;
      if (modifier.target === 'resistance' && modifier.value) resistances.add(modifier.value);
      if (modifier.target === 'conditionImmunity' && modifier.value) conditionImmunities.add(modifier.value);
      if (modifier.target === 'damageImmunity' && modifier.value) damageImmunities.add(modifier.value);
    }
    sheet.proficiencies.languages = [...languages];
    sheet.languages = [...languages];
    sheet.resistances = [...resistances];
    sheet.conditionImmunities = [...conditionImmunities];
    sheet.damageImmunities = [...damageImmunities];
    const senses = { ...(sheet.senses || {}) };
    for (const { grants } of grantSources) {
      for (const [id, distance] of Object.entries((grants && grants.senses) || {})) {
        senses[id] = Math.max(num(senses[id]), num(distance));
      }
    }
    sheet.senses = senses;
    const speedBonus = activeModifiers
      .filter((modifier) => modifier && modifier.target === 'speed')
      .reduce((sum, modifier) => sum + num(modifier.add), 0);
    if (speedBonus) {
      sheet.speed += speedBonus;
      sheet.derived.speed = sheet.speed;
    }
    sheet.flySpeed = activeModifiers
      .filter((modifier) => modifier && modifier.target === 'flySpeed')
      .reduce((max, modifier) => Math.max(
        max,
        modifier.value === 'speed' ? sheet.speed : num(modifier.value),
      ), 0);
    sheet.concentrationSaveBonus = activeModifiers
      .filter((modifier) => modifier && modifier.target === 'concentrationSave')
      .reduce((sum, modifier) => sum + num(modifier.add) + num(mods[modifier.addAbility]), 0);
  });

  // Highest spell level a class can prepare/cast on its OWN progression — the cap
  // for that class's preparable pool, so a multiclass low-level caster can't prepare
  // high-level spells off the COMBINED slot pool (a Cleric 1 / Wizard 5 prepares
  // Cleric spells only to L1). From the printed `spellSlots` when present, else the
  // caster-level heuristic (multiclass slot-table length) by casting fraction.
  const maxSpellLevelFor = (type, level, prog) => {
    const own = prog && Array.isArray(prog.spellSlots) ? prog.spellSlots : null;
    if (own) { let m = 0; for (let i = 0; i < own.length; i++) if (num(own[i]) > 0) m = i + 1; return m; }
    const d = type === 'full' ? 1 : type === 'half' ? 2 : type === 'third' ? 3 : 0;
    return d ? multiclassSlots(Math.ceil(num(level) / d), rs).length : 0;
  };

  // Spellcasting (MC-2/MC-3/SP-2/SP-4): per-class prepared limit + DC/attack,
  // plus the combined multiclass slot pool.
  step(() => {
    const per = [];
    const casters = [];
    for (const c of classes) {
      const sc = c.record && c.record.spellcasting;
      // a third-caster subclass (e.g. Eldritch Knight) carries spellcasting on the SUBCLASS
      const subRec = c.subclass && api && api.getItem ? api.getItem('subclass', c.subclass) : null;
      const eff = sc || (subRec && subRec.spellcasting) || null;
      if (!eff) continue;
      const ability = eff.ability;
      const mod = num(mods[ability]);
      const prog = progressionAt((subRec && subRec.progression) || (c.record && c.record.progression), c.level);
      // Stash the resolved progression row on the caster so the single-class slot
      // path can read its authoritative per-level `spellSlots` directly.
      casters.push({ type: eff.type, level: c.level, prog });
      // Warlock Pact Magic slots (short-rest, all one level) — derived by level; drives
      // the per-class prepare cap and a short-rest slot resource (below).
      const pact = eff.type === 'pact' ? pactMagic(c.level, rs) : null;
      // Wizard-style spellbook (SP-5): prepared is chosen from a LEARNED pool, not
      // the whole class list. The free-learn allotment isn't in the class table, so
      // derive it from the ruleset (2024: 6 spells at L1, +2 per Wizard level after).
      // Guidance only — copying from scrolls/other books grows the book beyond it.
      const prepares = eff.prepares || 'list';
      const sb = rs.constants.spellbook || {};
      const spellbookKnown = prepares === 'spellbook' ? num(sb.baseKnown, 6) + num(sb.knownPerLevel, 2) * (num(c.level, 1) - 1) : 0;
      per.push({
        classId: c.classId, level: num(c.level), ability, type: eff.type, prepares, ritual: !!eff.ritual,
        spellListClassId: eff.spellListClassId || c.classId,
        saveDC: 8 + pb + mod, spellAttack: pb + mod,
        preparedLimit: prog ? num(prog.preparedSpells, 0) : 0,
        cantripsKnown: prog ? num(prog.cantripsKnown, 0) : 0,
        spellbookKnown,
        maxSpellLevel: pact ? pact.level : maxSpellLevelFor(eff.type, c.level, prog),
        pact,
      });
    }

    const expandedSpellIds = [...new Set(featRecords.flatMap(({ record }) =>
      (record.grants && record.grants.spellList) || []))];
    for (const caster of per) caster.expandedSpellIds = expandedSpellIds.slice();

    // Slot pool (MC-2/MC-3). Two distinct rules:
    //  • SINGLE caster class → use the class's OWN printed per-level slot
    //    progression (`prog.spellSlots`) verbatim when the content provides it.
    //    The combined-caster-level heuristic diverges from the printed
    //    single-class table at high levels (e.g. Paladin L19 real = [4,3,3,1] but
    //    the heuristic gives [4,3,3,3,2]), so the authoritative table wins. When
    //    the (abbreviated/seed) content lacks `spellSlots`, fall back to the
    //    caster-level heuristic: ceil(level / fraction) rounded UP, so a 2024 L1
    //    half-caster (Paladin/Ranger gain Spellcasting at L1) still has slots and
    //    odd levels aren't undercounted. (Full casters: ceil(level/1) == level.)
    //  • MULTIPLE caster classes → the combined-caster-level rule (2024: full
    //    levels + half levels rounded UP + third levels rounded DOWN, see
    //    casterContribution) indexed into the ruleset's multiclass slot table.
    //  • PACT (Warlock) → never leveled slots: even if a book prints the pact
    //    column as `spellSlots`, reading it here would DOUBLE-COUNT (pactMagic
    //    already emits the pact pool separately), so pact skips ownSlots.
    const slotDivisor = (t) => (t === 'full' ? 1 : t === 'half' ? 2 : t === 'third' ? 3 : 0);
    let combinedCasterLevel;
    let slots = null;        // set directly when the single class table is used
    if (casters.length === 1) {
      const only = casters[0];
      const ownSlots = only.type !== 'pact' && only.prog && Array.isArray(only.prog.spellSlots) ? only.prog.spellSlots : null;
      const d = slotDivisor(only.type);
      // casterLevel reported for the UI/derive: the class's effective caster level.
      combinedCasterLevel = d ? Math.ceil(only.level / d) : 0;
      if (ownSlots) slots = ownSlots.slice();
    } else {
      combinedCasterLevel = casters.reduce((s, c) => s + casterContribution(c.type, c.level, rs), 0);
    }
    if (!slots) slots = multiclassSlots(combinedCasterLevel, rs);

    // Granted spells (SP-1/SP-2/SP-12): subclass always-prepared + feat grants +
    // species lineage. Each is provenance-tagged so the sheet can separate them
    // from the player's own picks and flag forced duplicates (SP-3). Names are
    // resolved from the compendium (falls back to the ref when names-only).
    const granted = [];
    const pendingChoices = [];
    const grantChoices = (cd && cd.grantChoices) || {};
    const grantCastingAbilities = (cd && cd.grantCastingAbilities) || {};
    const castingAbilityChoices = [];
    const castingChoiceKeys = new Set();
    const castingAbilityFor = (source, sourceGrants = null) => {
      const matching = sourceGrants ? null : grantSources.find((candidate) =>
        candidate.source.type === source.type
        && candidate.source.id === source.id
        && candidate.grants.castingAbility);
      const grants = sourceGrants || (matching && matching.grants);
      const declaration = grants && grants.castingAbility;
      if (!declaration) return null;
      if (declaration.fixed) return declaration.fixed;
      if (declaration.fromAbilityScoreIncrease) {
        for (const grant of Array.isArray(cd.abilityGrants) ? cd.abilityGrants : []) {
          if (!grant || !grant.source || grant.source.type !== 'feat') continue;
          const choiceId = String(grant.id || '').replace(/:featability$/, ':feat');
          if ((cd.featureChoices || {})[choiceId] !== source.id) continue;
          const selected = Object.keys(grant.assign || {}).find((ability) => num(grant.assign[ability]) > 0);
          if (selected) return selected;
        }
        return null;
      }
      const options = Array.isArray(declaration.choose) ? declaration.choose : [];
      const key = `${source.type}:${source.id}:${declaration.id || 'casting-ability'}`;
      const selected = options.includes(grantCastingAbilities[key])
        ? grantCastingAbilities[key]
        : null;
      if (!castingChoiceKeys.has(key)) {
        castingChoiceKeys.add(key);
        castingAbilityChoices.push({ key, source, options: options.slice(), selected });
      }
      return selected;
    };
    const addGrant = (ref, source, opts) => {
      if (!ref) return;
      const rec = api && api.getItem ? api.getItem('spell', ref) : null;
      granted.push({
        ref, name: rec ? rec.name : ref, level: rec ? num(rec.level) : null, school: rec ? rec.school : '',
        source,
        alwaysPrepared: !!(opts && opts.alwaysPrepared),
        free: (opts && opts.free) || null,
        castingAbility: (opts && opts.castingAbility) || null,
        castAtLevel: opts && opts.castAtLevel != null ? num(opts.castAtLevel) : null,
      });
    };
    // One grant entry: either FIXED (`ids`) or a CHOICE (`choose` + `from`). A
    // choice resolves the player's picks from cd.grantChoices[key] and exposes
    // the (possibly under-filled) choice on the sheet so the UI can render a
    // filtered picker (SP-10/SP-20 — Magic Initiate, Fey Touched's choose-1,
    // High Elf's wizard cantrip). `unlocked` gates by the source's level.
    const addGrantEntry = (sp, source, unlocked, sourceLevel = 0, sourceGrants = null) => {
      if (!unlocked) return;
      const castingAbility = castingAbilityFor(source, sourceGrants);
      let castAtLevel = null;
      for (const row of Array.isArray(sp.castAtLevelByLevel) ? sp.castAtLevelByLevel : []) {
        if (num(sourceLevel) >= num(row.level)) castAtLevel = num(row.castAtLevel);
      }
      if (Array.isArray(sp.ids) && sp.ids.length) {
        for (const ref of sp.ids) addGrant(ref, source, {
          alwaysPrepared: sp.alwaysPrepared, free: sp.free, castingAbility, castAtLevel,
        });
      } else if (num(sp.choose) > 0 && sp.id) {
        const key = source.type + ':' + source.id + ':' + sp.id;
        const explicit = Array.isArray(grantChoices[key])
          ? grantChoices[key].slice(0, num(sp.choose))
          : [];
        const picked = explicit.length || !sp.default ? explicit : [sp.default];
        for (const ref of picked) addGrant(ref, source, {
          alwaysPrepared: sp.alwaysPrepared, free: sp.free, castingAbility, castAtLevel,
        });
        pendingChoices.push({
          key,
          source,
          choose: num(sp.choose),
          spellLevel: sp.spellLevel == null ? null : num(sp.spellLevel),
          maxSpellLevel: sp.maxSpellLevel == null ? null : num(sp.maxSpellLevel),
          from: sp.from || {},
          default: sp.default || null,
          alwaysPrepared: !!sp.alwaysPrepared,
          picked: picked.slice(),
        });
      }
    };
    const unlockLevel = (sp) => num(sp.atLevel != null ? sp.atLevel : sp.level);
    for (const c of classes) {
      const subRec = c.subclass && api && api.getItem ? api.getItem('subclass', c.subclass) : null;
      for (const sp of (subRec && subRec.spells) || []) {
        addGrantEntry(sp, { type: 'subclass', id: c.subclass }, unlockLevel(sp) <= c.level, c.level);
      }
    }
    for (const source of grantSources) {
      for (const sp of source.grants.spells || []) {
        addGrantEntry(sp, source.source, unlockLevel(sp) <= source.level, source.level, source.grants);
      }
    }
    for (const { id: fid, record: frec } of featRecords) {
      const target = frec.grants && frec.grants.prepareSpellListOf;
      if (!target) continue;
      for (const { id: sourceId, record: sourceFeat } of featRecords) {
        if (target !== sourceFeat.category) continue;
        for (const ref of (sourceFeat.grants && sourceFeat.grants.spellList) || []) {
          addGrant(ref, { type: 'feat', id: sourceId }, {
            alwaysPrepared: true,
            castingAbility: castingAbilityFor({ type: 'feat', id: sourceId }),
          });
        }
      }
    }

    sheet.spellcasting = {
      perClass: per,
      casterLevel: combinedCasterLevel,
      slots,
      granted,
      pendingChoices,
      castingAbilityChoices,
    };
  });

  // Weapon Mastery slots (EQ-4): the best class count + Weapon Master feat.
  // A 2024-only subsystem — a ruleset without the capability zeroes the slots
  // (the data self-gates too: 2014 records carry no weaponMastery field).
  step(() => {
    let count = 0;
    if (rs.capabilities.weaponMastery !== false) {
      for (const c of classes) count = Math.max(count, num(c.record && c.record.weaponMastery && c.record.weaponMastery.count));
      const feats = Array.isArray(cd.feats) ? cd.feats.map((f) => (f && (f.featId || f.id || f))) : [];
      if (feats.includes('weapon-master')) count += 1;
    }
    sheet.weaponMastery = { slots: count, chosen: Array.isArray(cd.weaponMasteryChoices) ? cd.weaponMasteryChoices.slice() : [] };
  });

  // Weapons (EQ-5) + attunement (EQ-3): attack + damage for each equipped/ready
  // weapon (resolved from inventory refs/names), and the attunement tally.
  step(() => {
    const inv = Array.isArray(cd.inventory) ? cd.inventory : [];
    const profW = classWeaponProf(classes, sheet.proficiencies.weapons);
    const masterySet = new Set(cd.weaponMasteryChoices || []);
    const preferredAbility = activeModifiers
      .filter((modifier) => modifier && modifier.target === 'weaponAbility')
      .map((modifier) => modifier.ability)
      .find(Boolean);
    const resolveW = (it) => (it.ref && api && api.getItem && api.getItem('weapon', it.ref)) || (it.name && api && api.getItemByName && api.getItemByName('weapon', it.name)) || null;
    const weapons = [];
    let attuned = 0;
    for (const it of inv) {
      if (it.attuned) attuned++;
      const loc = it.location || 'pack';
      if (loc !== 'equipped' && loc !== 'ready') continue;
      const rec = resolveW(it);
      if (rec) weapons.push(computeWeaponAttack(rec, mods, pb, profW, masterySet, preferredAbility));
    }
    sheet.weapons = weapons;
    let attuneLimit = num(rs.constants.attunementLimit, 3);
    for (const c of classes) {
      for (const row of (c.record && c.record.attunementLimit) || []) {
        if (num(row.level, 1) <= c.level) attuneLimit = Math.max(attuneLimit, num(row.max, attuneLimit));
      }
    }
    sheet.attunement = { count: attuned, limit: attuneLimit, over: attuned > attuneLimit };
    if (attuned > attuneLimit) warn('Attuned to more than ' + attuneLimit + ' magic items (limit ' + attuneLimit + ')');
  });

  // Collected features (provenance-tagged) — feeds the Builder's level log.
  // Class features are derived from the per-class `feature` RECORDS (join:
  // classId + record level ≤ class level) — the progression[].features
  // name-strings are display labels, not identity (a drifted table once granted
  // the L18 Spell Mastery to a L2 wizard). Strings still grant whatever has no
  // record of that name in the class: the generic labels (ASI / Epic Boon /
  // "<Class> Subclass" / upgrade markers) and whole books that predate feature
  // records. A string whose name is a record's at another
  // level is drift and grants nothing. Option-pool records (category:
  // metamagic / maneuver / invocation) are choice fodder, never auto-granted.
  step(() => {
    const feats = [];
    const norm = (s) => String(s || '').trim().toLowerCase();
    for (const c of classes) {
      const records = (api && api.listFeatures ? api.listFeatures({ classId: c.classId }) : [])
        .filter((f) => f && !f.subclassId && !f.category && f.level != null);
      const owned = new Set(records.map((f) => norm(f.name)));
      const recsAt = new Map();   // level → class-feature records gained there
      for (const f of records) {
        const lv = num(f.level);
        if (!recsAt.has(lv)) recsAt.set(lv, []);
        recsAt.get(lv).push(f);
      }
      const rowsAt = new Map();   // level → printed progression row
      for (const row of (c.record && c.record.progression) || []) rowsAt.set(num(row.level), row);
      const levels = [...new Set([...recsAt.keys(), ...rowsAt.keys()])].sort((a, b) => a - b);
      for (const lv of levels) {
        if (lv > c.level) continue;
        const source = { type: 'class', id: c.classId, level: lv };
        const recs = recsAt.get(lv) || [];
        const row = rowsAt.get(lv);
        const taken = new Set();
        // Walk the printed row first so the book's order is kept: a string naming
        // a record AT THIS level emits that record (repeat names each take one)...
        for (const s of (row && row.features) || []) {
          const hit = recs.find((f) => !taken.has(f.id) && norm(f.name) === norm(s));
          if (hit) { taken.add(hit.id); feats.push({ id: hit.id, name: hit.name, source }); }
          else if (!owned.has(norm(s))) feats.push({ id: s, source });   // recordless label → verbatim
        }
        // ...then records the row forgot still grant (records are the identity).
        for (const f of recs) if (!taken.has(f.id)) feats.push({ id: f.id, name: f.name, source });
      }
      const subRec = c.subclass && api && api.getItem ? api.getItem('subclass', c.subclass) : null;
      for (const f of (subRec && subRec.features) || []) if (num(f.level) <= c.level) feats.push({ id: f.id, name: f.name, source: { type: 'subclass', id: c.subclass, level: f.level } });
    }
    sheet.features = feats;
  });

  // Resource pools with recharge (FE-2/FE-3) — the single tracker model the sheet
  // renders and the Rest wizard resets. Emits FOUR kinds, each carrying a
  // structured `recharge: [{ on:'short'|'long', amount:'full'|<int>|'halfLevel'|
  // {abilityMod} }]` so the wizard knows WHAT resets on which rest and by HOW MUCH:
  //   • pool    — class resources (Rage / Focus / Channel Divinity…), from
  //               each class's `classResources` (max = progression table / per-level
  //               multiple / ability modifier / fixed).
  //   • hitdice — one per die size across the build (max = summed class levels).
  //   • slot    — spell slots per level (from spellcasting.slots).
  //   • charge  — free/limited casts granted by feats/species/subclass features
  //               (from spellcasting.granted[].free, e.g. Fey Touched's 1/long).
  step(() => {
    const abilMod = (a) => (sheet.abilities && sheet.abilities[a] ? num(sheet.abilities[a].mod, 0) : 0);
    const resolveMax = (res, lvl) => {
      if (Array.isArray(res.progression) && res.progression.length) {
        let m = 0;
        for (const row of res.progression) if (num(row.level, 1) <= lvl) m = num(row.max, m);
        return m;
      }
      if (res.perLevel != null) return Math.max(0, Math.floor(num(res.perLevel) * lvl));
      if (res.proficiencyBonus) return pb;
      if (res.abilityMod) return Math.max(num(res.min, 1), abilMod(res.abilityMod));
      if (res.fixed != null) return Math.max(0, num(res.fixed));
      return 0;
    };
    // Normalize recharge into [{on, amount}]. A bare string ('long') means
    // "resets to full on that rest"; an array is taken as-is (defaulting amount).
    const normRecharge = (r, level = Infinity) => {
      if (typeof r === 'string') return [{ on: r, amount: 'full' }];
      if (Array.isArray(r)) {
        return r
          .filter((x) => x && x.on && num(x.minLevel, 1) <= level)
          .map((x) => ({ on: String(x.on), amount: x.amount == null ? 'full' : x.amount }));
      }
      return [{ on: 'long', amount: 'full' }];
    };
    const ORD = ['1st', '2nd', '3rd', '4th', '5th', '6th', '7th', '8th', '9th'];
    const resources = [];

    // 1. Class + subclass resource pools (both resolved at the class's level).
    for (const c of classes) {
      const subRec = c.subclass && api && api.getItem ? api.getItem('subclass', c.subclass) : null;
      const pools = []
        .concat(((c.record && c.record.classResources) || []).map((res) => [res, { type: 'class', id: c.classId, level: c.level }]))
        .concat(((subRec && subRec.classResources) || []).map((res) => [res, { type: 'subclass', id: c.subclass, level: c.level }]));
      for (const [res, source] of pools) {
        if (!res || !res.key) continue;
        if (num(res.minLevel, 1) > c.level) continue;
        const resourceId = `${res.key} ${res.name || ''}`.toLowerCase();
        const hasDerivedPactSlots = source.type === 'class' &&
          ((sheet.spellcasting && sheet.spellcasting.perClass) || [])
            .some((entry) => entry.classId === source.id && entry.pact);
        if (hasDerivedPactSlots && /\bpact[- ]slots?\b/.test(resourceId)) continue;
        const max = resolveMax(res, c.level);
        if (max <= 0) continue;
        resources.push({
          key: String(res.key), name: res.name || String(res.key), max, kind: 'pool',
          recharge: normRecharge(res.recharge, c.level), source,
        });
      }
    }
    for (const source of grantSources) {
      for (const res of source.grants.resources || []) {
        if (!res || !res.key || num(res.minLevel, 1) > source.level) continue;
        const max = resolveMax(res, source.level);
        if (max <= 0) continue;
        resources.push({
          key: String(res.key),
          name: res.name || String(res.key),
          max,
          kind: 'pool',
          recharge: normRecharge(res.recharge, source.level),
          source: source.source,
        });
      }
    }
    // 2. Hit Dice — aggregate by die size. 2024 Long Rest: "You regain all
    // lost Hit Points and all spent Hit Point Dice" — ALL of them; a 2014
    // ruleset sets rest.longRestHitDice to 'half' (regain up to half your
    // total), which maps onto the recharge vocabulary's 'halfLevel' amount.
    const hdAmount = rs.constants.rest && rs.constants.rest.longRestHitDice !== 'all' ? 'halfLevel' : 'full';
    const byDie = {};
    for (const c of classes) { const d = c.record && c.record.hitDie; if (d) byDie[d] = (byDie[d] || 0) + c.level; }
    for (const die of Object.keys(byDie)) {
      resources.push({
        key: 'hit-dice-' + die, name: 'Hit Dice (' + die + ')', max: byDie[die], kind: 'hitdice', die,
        recharge: [{ on: 'long', amount: hdAmount }], source: { type: 'class' },
      });
    }
    // 3. Spell slots (leveled). Long rest → full.
    const slots = (sheet.spellcasting && sheet.spellcasting.slots) || [];
    slots.forEach((n, i) => {
      if (num(n) > 0) resources.push({
        key: 'slot-' + (i + 1), name: 'Spell Slots (' + (ORD[i] || (i + 1) + 'th') + ')', max: num(n), kind: 'slot',
        recharge: [{ on: 'long', amount: 'full' }], source: { type: 'spellcasting' },
      });
    });
    for (const selected of Array.isArray(cd.feats) ? cd.feats : []) {
      const featId = selected && (selected.featId || selected.id || selected);
      const feat = featId && api && api.getItem ? api.getItem('feat', featId) : null;
      const slot = feat && feat.grants && feat.grants.spellSlot;
      if (!slot) continue;
      const levelRule = slot.level || {};
      const divisor = Math.max(1, num(levelRule.divisor, 1));
      const rawLevel = totalLevel / divisor;
      const rounded = levelRule.round === 'down' ? Math.floor(rawLevel) : Math.ceil(rawLevel);
      const level = Math.max(num(levelRule.min, 1), Math.min(num(levelRule.max, 9), rounded));
      resources.push({
        key: `feat-slot-${featId}`,
        name: `${feat.name || featId} (${ORD[level - 1] || `${level}th`})`,
        max: Math.max(1, num(slot.count, 1)),
        kind: 'slot',
        level,
        restriction: slot.restriction || null,
        recharge: normRecharge(slot.recharge, totalLevel),
        source: { type: 'feat', id: featId },
      });
    }
    // Pact Magic slots (Warlock): a small pool ALL at one level, SHORT-rest recharge.
    for (const p of (sheet.spellcasting && sheet.spellcasting.perClass) || []) {
      if (p.pact && num(p.pact.slots) > 0) resources.push({
        key: 'pact-slot', name: 'Pact Slots (' + (ORD[p.pact.level - 1] || p.pact.level + 'th') + ')', max: num(p.pact.slots), kind: 'slot',
        recharge: [{ on: 'short', amount: 'full' }], source: { type: 'pactMagic', id: p.classId },
      });
    }
    // 4. Granted free/limited casts (feat / species / subclass) → charges.
    const parseFreq = (frequency) => {
      const match = /(\d+)\s*\/\s*(shortOrLong|short|long)/i.exec(String(frequency || ''));
      if (!match) return { max: 1, on: ['long'] };
      const rest = match[2].toLowerCase();
      return {
        max: num(match[1], 1),
        on: rest === 'shortorlong' ? ['short', 'long'] : [rest],
      };
    };
    for (const g of (sheet.spellcasting && sheet.spellcasting.granted) || []) {
      if (!g.free) continue;
      const fq = parseFreq(g.free);
      resources.push({
        key: 'charge-' + (g.ref || g.name), name: (g.name || g.ref) + ' (free cast)', max: fq.max, kind: 'charge',
        recharge: fq.on.map((on) => ({ on, amount: 'full' })), source: g.source || { type: 'spell' },
      });
    }
    sheet.resources = resources;
  });

  return { sheet, warnings };
}
