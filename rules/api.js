import * as Engine from './engine.js';
import * as Builder from './builder.js';
import {
  RULES_DATA_CONTRACT,
  RULES_DATA_CONTRACT_VERSION,
  inspectRulesDataHandle,
} from '../contract/rules-data.js';

export const RULES_ENGINE_CONTRACT = 'dnd5e.rules-engine';
export const RULES_ENGINE_CONTRACT_VERSION = '2.0.0';

const clone = value => {
  if (value == null) return value;
  if (typeof structuredClone === 'function') return structuredClone(value);
  return JSON.parse(JSON.stringify(value));
};

export function makeRulesApi(getRulesDataHandle, { isDisposed = () => false } = {}) {
  const inspectedHandles = new WeakMap();
  const safeApis = new WeakMap();
  const active = () => {
    if (isDisposed()) throw new Error('D&D engine service has been disposed');
  };
  const context = () => {
    active();
    let handle = null;
    try { handle = getRulesDataHandle?.() || null; } catch (_) {}
    if (handle && (typeof handle === 'object' || typeof handle === 'function')) {
      const cached = inspectedHandles.get(handle);
      if (cached) return cached;
      const inspected = inspectRulesDataHandle(handle);
      const current = inspected.available
        ? Object.freeze({ ...inspected, api: safeProviderApi(inspected.api, safeApis) })
        : inspected;
      if (inspected.available) inspectedHandles.set(handle, current);
      return current;
    }
    return inspectRulesDataHandle(handle);
  };
  const data = () => context().api;
  const ruleset = () => context().ruleset;
  const list = (method, ...args) => data()?.[method]?.(...args) || [];
  const item = (method, ...args) => data()?.[method]?.(...args) || null;

  const hydrate = decisions => {
    active();
    const current = context();
    if (!current.available) {
      return clone(hydrateWithoutRulesData(decisions, current.status));
    }
    const normalized = Builder.normalizeBuilderDecisions(clone(decisions || {}), current.api, current.ruleset);
    return clone(Engine.hydrate(normalized, current.api, current.ruleset));
  };

  const api = {
    apiVersion: 2,
    getAvailability: () => {
      const current = context();
      return Object.freeze({ available: current.available, status: current.status, errors: current.errors });
    },
    getContextIdentity: () => {
      const current = context();
      return Object.freeze({
        engineContract: RULES_ENGINE_CONTRACT,
        engineContractVersion: RULES_ENGINE_CONTRACT_VERSION,
        rulesDataContract: RULES_DATA_CONTRACT,
        rulesDataContractVersion: RULES_DATA_CONTRACT_VERSION,
        available: current.available,
        status: current.status,
        ...current.identity,
      });
    },
    listClasses: () => list('listClasses'),
    listSubclasses: classId => list('listSubclasses', classId),
    listFeatures: query => list('listFeatures', query),
    getFeature: id => item('getFeature', id),
    listSpecies: () => list('listSpecies'),
    listBackgrounds: () => list('listBackgrounds'),
    listFeats: options => list('listFeats', options),
    listSpells: query => list('listSpells', query),
    listEquipment: query => list('listEquipment', query),
    listArmor: () => list('listArmor'),
    listWeapons: () => list('listWeapons'),
    listSkills: () => list('listSkills'),
    getItem: (kind, id) => item('getItem', kind, id),
    getItemByName: (kind, name) => item('getItemByName', kind, name),
    getRecords: kind => list('getRecords', kind),
    resolveReference: (kind, id, mode = 'view') => {
      active();
      return data()?.resolveReference?.(kind, id, mode) || null;
    },
    getBuilderPlan: decisions => {
      const current = context();
      return current.available
        ? clone(Builder.getBuilderPlan(clone(decisions || {}), current.api, current.ruleset))
        : null;
    },
    applyBuilderChoice: (decisions, change) => {
      const current = context();
      return current.available
        ? clone(Builder.applyBuilderChoice(clone(decisions || {}), clone(change || {}), current.api, current.ruleset))
        : clone(decisions || {});
    },
    reconcileBuilderDecisions: decisions => {
      const current = context();
      return current.available
        ? clone(Builder.reconcileBuilderDecisions(clone(decisions || {}), current.api, current.ruleset))
        : clone(decisions || {});
    },
    hydrate,
    derive: Object.freeze({
      abilityMod: Engine.abilityMod,
      proficiencyBonus: Engine.proficiencyBonus,
      hitDieAverage: Engine.hitDieAvg,
      scrollCopyCost: level => ruleset() ? Engine.scrollCopyCost(level, ruleset()) : null,
      pointBuyCost: score => ruleset() ? Engine.pointCost(score, ruleset()) : null,
      pointsSpent: scores => ruleset() ? Engine.pointsSpent(scores, ruleset()) : null,
      featAbilityCap: feat => ruleset() ? Engine.featAbilityCap(feat, ruleset()) : null,
      featAsiFrom: Engine.featAsiFrom,
      multiclassSlots: casterLevel => ruleset() ? Engine.multiclassSlots(casterLevel, ruleset()) : [],
      initiative: decisions => hydrate(decisions).sheet.derived.initiative,
      maxHp: decisions => hydrate(decisions).sheet.derived.maxHp,
      armorClass: decisions => hydrate(decisions).sheet.derived.armorClass,
      saveDC: (abilityScore, totalLevel) => 8 + Engine.proficiencyBonus(totalLevel) + Engine.abilityMod(abilityScore),
    }),
  };
  return Object.freeze(api);
}

function hydrateWithoutRulesData(decisions, status) {
  const source = decisions || {};
  const base = source.baseStats || source.abilities || {};
  const abilities = {};
  for (const ability of Engine.ABILITIES) {
    const score = Engine.num(base[ability], 10);
    abilities[ability] = { base: score, score, mod: Engine.abilityMod(score), bonus: 0 };
  }
  const totalLevel = Array.isArray(source.classes) && source.classes.length
    ? source.classes.reduce((sum, entry) => sum + Math.max(0, Engine.num(entry?.level)), 0)
    : Math.max(1, Engine.num(source.level, 1));
  return {
    sheet: {
      abilities,
      totalLevel,
      derived: {
        proficiencyBonus: Engine.proficiencyBonus(totalLevel),
        initiative: abilities.DEX.mod,
      },
      proficiencies: { saves: {}, skills: {}, armor: [], weapons: [], tools: [], languages: [] },
      features: [],
    },
    warnings: [`Rules data unavailable (${status}); edition-dependent computation was skipped.`],
  };
}

function safeProviderApi(raw, cache) {
  if (!raw || typeof raw !== 'object') return null;
  const cached = cache.get(raw);
  if (cached) return cached;

  const safe = {};
  for (const method of [
    'listClasses', 'listSubclasses', 'listFeatures', 'listSpecies',
    'listBackgrounds', 'listFeats', 'listSpells', 'listEquipment',
    'listArmor', 'listWeapons', 'listSkills', 'getRecords',
  ]) {
    safe[method] = (...args) => safeListCall(raw, method, args);
  }
  for (const method of ['getFeature', 'getItem', 'getItemByName']) {
    safe[method] = (...args) => safeItemCall(raw, method, args);
  }
  safe.resolveReference = (...args) => safeItemCall(raw, 'resolveReference', args);
  const frozen = Object.freeze(safe);
  cache.set(raw, frozen);
  return frozen;
}

function safeListCall(raw, method, args) {
  try {
    const fn = raw[method];
    if (typeof fn !== 'function') return [];
    const result = fn.apply(raw, args);
    return Array.isArray(result) ? clone(result) : [];
  } catch (_) {
    return [];
  }
}

function safeItemCall(raw, method, args) {
  try {
    const fn = raw[method];
    if (typeof fn !== 'function') return null;
    const result = fn.apply(raw, args);
    return result && typeof result === 'object' && !Array.isArray(result) ? clone(result) : null;
  } catch (_) {
    return null;
  }
}
