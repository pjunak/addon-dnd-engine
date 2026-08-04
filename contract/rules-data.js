import { DEFAULT_RULESET } from '../rules/ruleset.js';

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

const isObject = value => !!value && typeof value === 'object' && !Array.isArray(value);
const valueAt = (value, path) => path.split('.').reduce((current, key) => current?.[key], value);
const safeId = value => /^[a-z0-9][a-z0-9._-]{0,79}$/.test(String(value || ''));

export function validateRuleset(record) {
  const errors = [];
  if (!isObject(record)) return Object.freeze({ ok: false, errors: Object.freeze(['ruleset is unavailable']) });

  const rulesetId = record.rulesetId || record.id;
  if (!safeId(rulesetId)) errors.push('rulesetId must be a stable lowercase identifier');
  if (!Number.isInteger(record.rulesetVersion) || record.rulesetVersion < 1) {
    errors.push('rulesetVersion must be a positive integer');
  }
  if (typeof record.edition !== 'string' || !record.edition.trim()) errors.push('edition is required');

  if (record.extends !== undefined && record.extends !== DEFAULT_RULESET.rulesetId) {
    errors.push(`unsupported ruleset base "${String(record.extends)}"`);
  }
  if (record.extends === undefined) {
    for (const path of RULESET_KEYS) {
      if (valueAt(record, path) === undefined) errors.push(`complete ruleset is missing ${path}`);
    }
  }

  const slots = valueAt(record.extends ? { ...DEFAULT_RULESET, ...record } : record, 'constants.multiclassSlots');
  if (slots !== undefined && (!isObject(slots) || Array.from({ length: 20 }, (_, index) => slots[index + 1])
    .some(row => !Array.isArray(row) || !row.length || row.some(value => !Number.isInteger(value) || value < 0)))) {
    errors.push('constants.multiclassSlots must contain valid rows 1 through 20');
  }

  return Object.freeze({
    ok: errors.length === 0,
    errors: Object.freeze(errors),
    identity: Object.freeze({
      rulesetId: String(rulesetId || ''),
      rulesetVersion: Number(record.rulesetVersion) || 0,
      edition: String(record.edition || ''),
    }),
  });
}

export function inspectRulesDataHandle(handle) {
  const provider = handle?.provider;
  const api = handle?.api;
  if (!provider || !isObject(api)) return unavailable('missing', null, ['No rules-data service is selected.']);
  if (provider.contract !== RULES_DATA_CONTRACT || !String(provider.contractVersion || '').startsWith('1.')) {
    return unavailable('incompatible', provider, ['The selected service does not implement dnd5e.rules-data v1.']);
  }

  const errors = [];
  if (api.apiVersion !== 1) errors.push('rules-data apiVersion must be 1');
  for (const method of REQUIRED_RULES_DATA_METHODS) {
    if (typeof api[method] !== 'function') errors.push(`rules-data method ${method}() is required`);
  }
  if (errors.length) return unavailable('incompatible', provider, errors);

  let ruleset;
  try {
    ruleset = api.getRuleset();
  } catch (error) {
    return unavailable('error', provider, [error?.message || 'getRuleset() failed']);
  }
  if (!ruleset) return unavailable('pending', provider, ['Rules data is still loading.']);

  const validation = validateRuleset(ruleset);
  if (!validation.ok) return unavailable('incompatible', provider, validation.errors);
  return Object.freeze({
    available: true,
    status: 'ready',
    api,
    ruleset,
    identity: Object.freeze({
      providerAddonId: provider.addonId,
      providerAddonVersion: provider.addonVersion || '',
      providerContractVersion: provider.contractVersion,
      contentRevision: provider.contentRevision || '',
      ...validation.identity,
    }),
    errors: Object.freeze([]),
  });
}

function unavailable(status, provider, errors) {
  return Object.freeze({
    available: false,
    status,
    api: null,
    ruleset: null,
    identity: Object.freeze({
      providerAddonId: provider?.addonId || '',
      providerAddonVersion: provider?.addonVersion || '',
      providerContractVersion: provider?.contractVersion || '',
      contentRevision: provider?.contentRevision || '',
      rulesetId: '',
      rulesetVersion: 0,
      edition: '',
    }),
    errors: Object.freeze([...errors]),
  });
}
