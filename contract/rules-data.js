import { DEFAULT_RULESET, resolveRuleset } from '../rules/ruleset.js';

export const RULES_DATA_CONTRACT = 'dnd5e.rules-data';
export const RULES_DATA_CONTRACT_VERSION = '1.0.0';

export const REQUIRED_RULES_DATA_METHODS = Object.freeze([
  'listClasses', 'listSubclasses', 'listFeatures', 'getFeature',
  'listSpecies', 'listBackgrounds', 'listFeats', 'listSpells',
  'listEquipment', 'listArmor', 'listWeapons', 'listSkills',
  'getItem', 'getItemByName', 'getRecords', 'getRuleset',
]);

const RULESET_KEYS = Object.freeze([
  'constants.abilityCap', 'constants.abilityCapHard',
  'constants.attunementLimit', 'constants.scrollCopyGpPerLevel',
  'constants.pointBuy', 'constants.asi', 'constants.multiclassSlots',
  'constants.casterFractions', 'constants.pactMagic',
  'constants.spellbook', 'constants.rest',
  'capabilities.weaponMastery', 'capabilities.epicBoons',
  'capabilities.backgroundAsi', 'capabilities.speciesAsi',
  'capabilities.originFeats',
]);

const FORBIDDEN_KEYS = new Set(['__proto__', 'constructor', 'prototype']);
const MAX_RULESET_DEPTH = 32;
const MAX_RULESET_NODES = 10_000;
const isObject = value => !!value && typeof value === 'object' && !Array.isArray(value);
const valueAt = (value, path) => path.split('.').reduce((current, key) => current?.[key], value);
const safeId = value => /^[a-z0-9][a-z0-9._-]{0,79}$/.test(String(value || ''));
const isFiniteNumber = value => typeof value === 'number' && Number.isFinite(value);
const isIntegerIn = (value, min, max = Number.MAX_SAFE_INTEGER) => Number.isInteger(value) && value >= min && value <= max;

export function validateRuleset(record) {
  const analysis = analyzeRuleset(record);
  return Object.freeze({
    ok: analysis.errors.length === 0,
    errors: analysis.errors,
    identity: analysis.identity,
  });
}

export function inspectRulesDataHandle(handle) {
  let provider;
  let api;
  try {
    provider = handle?.provider;
    api = handle?.api;
  } catch (error) {
    return unavailable('error', null, [messageOf(error, 'Rules-data service handle could not be read.')]);
  }
  if (!provider || !isObject(api)) return unavailable('missing', null, ['No rules-data service is selected.']);

  try {
    if (provider.contract !== RULES_DATA_CONTRACT || !String(provider.contractVersion || '').startsWith('1.')) {
      return unavailable('incompatible', provider, ['The selected service does not implement dnd5e.rules-data v1.']);
    }

    const errors = [];
    if (api.apiVersion !== 1) errors.push('rules-data apiVersion must be 1');
    for (const method of REQUIRED_RULES_DATA_METHODS) {
      if (typeof api[method] !== 'function') errors.push(`rules-data method ${method}() is required`);
    }
    if (errors.length) return unavailable('incompatible', provider, errors);
  } catch (error) {
    return unavailable('error', provider, [messageOf(error, 'Rules-data service metadata could not be read.')]);
  }

  let ruleset;
  try {
    ruleset = api.getRuleset();
  } catch (error) {
    return unavailable('error', provider, [messageOf(error, 'getRuleset() failed')]);
  }
  if (!ruleset) return unavailable('pending', provider, ['Rules data is still loading.']);

  const analysis = analyzeRuleset(ruleset);
  if (analysis.errors.length) return unavailable('incompatible', provider, analysis.errors);
  return Object.freeze({
    available: true,
    status: 'ready',
    api,
    ruleset: analysis.ruleset,
    identity: Object.freeze({
      providerAddonId: provider.addonId,
      providerAddonVersion: provider.addonVersion || '',
      providerContractVersion: provider.contractVersion,
      contentRevision: provider.contentRevision || '',
      ...analysis.identity,
    }),
    errors: Object.freeze([]),
  });
}

function analyzeRuleset(record) {
  const errors = [];
  let snapshot = null;
  try {
    snapshot = clonePlain(record);
  } catch (error) {
    errors.push(messageOf(error, 'ruleset must be a finite plain-data record'));
  }
  if (!isObject(snapshot)) {
    if (!errors.length) errors.push('ruleset is unavailable');
    return analysisResult(null, errors);
  }

  const rulesetId = snapshot.rulesetId || snapshot.id;
  if (!safeId(rulesetId)) errors.push('rulesetId must be a stable lowercase identifier');
  if (!isIntegerIn(snapshot.rulesetVersion, 1)) errors.push('rulesetVersion must be a positive integer');
  if (typeof snapshot.edition !== 'string' || !snapshot.edition.trim()) errors.push('edition is required');

  if (snapshot.extends !== undefined && snapshot.extends !== DEFAULT_RULESET.rulesetId) {
    errors.push(`unsupported ruleset base "${String(snapshot.extends)}"`);
  }
  if (snapshot.extends === undefined) {
    for (const path of RULESET_KEYS) {
      if (valueAt(snapshot, path) === undefined) errors.push(`complete ruleset is missing ${path}`);
    }
  }

  const candidate = snapshot.extends === DEFAULT_RULESET.rulesetId ? resolveRuleset(snapshot) : snapshot;
  validateConstants(candidate.constants, errors);
  validateCapabilities(candidate.capabilities, candidate.constants, errors);

  return analysisResult(errors.length ? null : deepFreeze(candidate), errors, {
    rulesetId: String(rulesetId || ''),
    rulesetVersion: Number(snapshot.rulesetVersion) || 0,
    edition: String(snapshot.edition || ''),
  });
}

function validateConstants(constants, errors) {
  if (!isObject(constants)) {
    errors.push('constants must be an object');
    return;
  }
  if (!isIntegerIn(constants.abilityCap, 1)) errors.push('constants.abilityCap must be a positive integer');
  if (!isIntegerIn(constants.abilityCapHard, constants.abilityCap || 1)) {
    errors.push('constants.abilityCapHard must be an integer at least as large as abilityCap');
  }
  if (!isIntegerIn(constants.attunementLimit, 0)) errors.push('constants.attunementLimit must be a non-negative integer');
  if (!isFiniteNumber(constants.scrollCopyGpPerLevel) || constants.scrollCopyGpPerLevel < 0) {
    errors.push('constants.scrollCopyGpPerLevel must be a non-negative finite number');
  }
  validatePointBuy(constants.pointBuy, errors);
  validateAsi(constants.asi, errors);
  validateMulticlassSlots(constants.multiclassSlots, errors);
  validateCasterFractions(constants.casterFractions, errors);
  validatePactMagic(constants.pactMagic, errors);
  if (!isObject(constants.spellbook)
    || !isIntegerIn(constants.spellbook.baseKnown, 0)
    || !isIntegerIn(constants.spellbook.knownPerLevel, 0)) {
    errors.push('constants.spellbook must define non-negative integer baseKnown and knownPerLevel values');
  }
  if (!isObject(constants.rest) || !['all', 'half'].includes(constants.rest.longRestHitDice)) {
    errors.push('constants.rest.longRestHitDice must be "all" or "half"');
  }
}

function validatePointBuy(pointBuy, errors) {
  if (!isObject(pointBuy)
    || !isIntegerIn(pointBuy.budget, 0)
    || !isIntegerIn(pointBuy.min, 0)
    || !isIntegerIn(pointBuy.max, pointBuy.min || 0)
    || !isObject(pointBuy.cost)) {
    errors.push('constants.pointBuy must define a valid budget, score range, and cost table');
    return;
  }
  for (let score = pointBuy.min; score <= pointBuy.max; score += 1) {
    if (!isFiniteNumber(pointBuy.cost[score]) || pointBuy.cost[score] < 0) {
      errors.push(`constants.pointBuy.cost must define a non-negative finite cost for score ${score}`);
      break;
    }
  }
}

function validateAsi(asi, errors) {
  if (!isObject(asi)
    || !isIntegerIn(asi.budget, 0)
    || !isIntegerIn(asi.perMax, 0)
    || !isIntegerIn(asi.bgBudget, 0)
    || !isIntegerIn(asi.bgPerMax, 0)
    || !Array.isArray(asi.baseLevels)
    || asi.baseLevels.some(level => !isIntegerIn(level, 1))
    || new Set(asi.baseLevels).size !== asi.baseLevels.length) {
    errors.push('constants.asi must define valid budgets and unique positive baseLevels');
  }
}

function validateMulticlassSlots(slots, errors) {
  const valid = isObject(slots) && Array.from({ length: 20 }, (_, index) => slots[index + 1])
    .every(row => Array.isArray(row)
      && row.length > 0
      && row.every(value => isIntegerIn(value, 0)));
  if (!valid) errors.push('constants.multiclassSlots must contain valid rows 1 through 20');
}

function validateCasterFractions(fractions, errors) {
  if (!isObject(fractions) || !['up', 'down'].includes(fractions.half) || !['up', 'down'].includes(fractions.third)) {
    errors.push('constants.casterFractions must define half and third as "up" or "down"');
  }
}

function validatePactMagic(pactMagic, errors) {
  const tiers = pactMagic?.tiers;
  const validTiers = Array.isArray(tiers)
    && tiers.length > 0
    && tiers.every(tier => isObject(tier) && isIntegerIn(tier.level, 1) && isIntegerIn(tier.slots, 0))
    && tiers.every((tier, index) => index === 0 || tier.level > tiers[index - 1].level);
  if (!isObject(pactMagic) || !validTiers || !isIntegerIn(pactMagic.slotLevelCap, 1)) {
    errors.push('constants.pactMagic must define ascending positive levels, non-negative slots, and a positive slotLevelCap');
  }
}

function validateCapabilities(capabilities, constants, errors) {
  if (!isObject(capabilities)) {
    errors.push('capabilities must be an object');
    return;
  }
  for (const key of ['weaponMastery', 'backgroundAsi', 'speciesAsi', 'originFeats']) {
    if (typeof capabilities[key] !== 'boolean') errors.push(`capabilities.${key} must be boolean`);
  }
  const boons = capabilities.epicBoons;
  if (boons === false) return;
  if (!isObject(boons)
    || !isIntegerIn(boons.atLevel, 1)
    || !isIntegerIn(boons.abilityCap, constants?.abilityCap || 1, constants?.abilityCapHard || Number.MAX_SAFE_INTEGER)) {
    errors.push('capabilities.epicBoons must be false or define a positive level and an ability cap within the ruleset limits');
  }
}

function clonePlain(value, state = { nodes: 0 }, depth = 0) {
  state.nodes += 1;
  if (state.nodes > MAX_RULESET_NODES) throw new Error('ruleset exceeds the maximum size');
  if (depth > MAX_RULESET_DEPTH) throw new Error('ruleset exceeds the maximum nesting depth');
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return value;
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) throw new Error('ruleset contains a non-finite number');
    return value;
  }
  if (Array.isArray(value)) return value.map(item => clonePlain(item, state, depth + 1));
  if (!isObject(value)) throw new Error('ruleset must contain only plain data');
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) throw new Error('ruleset must contain only plain objects');
  const copy = {};
  for (const key of Object.keys(value)) {
    if (FORBIDDEN_KEYS.has(key)) throw new Error(`ruleset contains forbidden key "${key}"`);
    copy[key] = clonePlain(value[key], state, depth + 1);
  }
  return copy;
}

function deepFreeze(value) {
  if (!value || typeof value !== 'object' || Object.isFrozen(value)) return value;
  for (const child of Object.values(value)) deepFreeze(child);
  return Object.freeze(value);
}

function analysisResult(ruleset, errors, identity = {}) {
  return Object.freeze({
    ruleset,
    errors: Object.freeze([...errors]),
    identity: Object.freeze({
      rulesetId: identity.rulesetId || '',
      rulesetVersion: identity.rulesetVersion || 0,
      edition: identity.edition || '',
    }),
  });
}

function messageOf(error, fallback) {
  return typeof error?.message === 'string' && error.message ? error.message : fallback;
}

function unavailable(status, provider, errors) {
  let identity;
  try {
    identity = {
      providerAddonId: provider?.addonId || '',
      providerAddonVersion: provider?.addonVersion || '',
      providerContractVersion: provider?.contractVersion || '',
      contentRevision: provider?.contentRevision || '',
      rulesetId: '',
      rulesetVersion: 0,
      edition: '',
    };
  } catch (_) {
    identity = {
      providerAddonId: '', providerAddonVersion: '', providerContractVersion: '', contentRevision: '',
      rulesetId: '', rulesetVersion: 0, edition: '',
    };
  }
  return Object.freeze({
    available: false,
    status,
    api: null,
    ruleset: null,
    identity: Object.freeze(identity),
    errors: Object.freeze([...errors]),
  });
}
