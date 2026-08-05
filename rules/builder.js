import { ABILITIES, featAsiFrom, num } from './engine.js';
import { requireRuleset } from './ruleset.js';

export function getBuilderPlan(decisions, api, ruleset) {
  const profile = requireRuleset(ruleset);
  const model = builderModel(decisions || {}, api);
  const classChoices = collectClassChoices(model.classes, api, profile);
  const creationChoices = collectCreationChoices(decisions || {}, api);
  const creationAbilityChoices = collectCreationAbilityChoices(decisions || {}, api, profile);
  return {
    schemaVersion: 1,
    edition: profile.edition,
    baseStats: model.baseStats,
    classes: model.classes,
    pointBuy: { ...profile.constants.pointBuy },
    abilityScoreRange: {
      min: 1,
      max: num(profile.constants.abilityCapHard, profile.constants.abilityCap),
    },
    classChoices,
    creationChoices,
    creationAbilityChoices,
  };
}

export function normalizeBuilderDecisions(decisions, api, ruleset) {
  const source = decisions || {};
  const plan = getBuilderPlan(source, api, ruleset);
  return {
    ...source,
    saveProf: { ...(source.manualSaveProf || source.saveProf || {}) },
    classes: plan.classes,
    baseStats: plan.baseStats,
    ...resolveChoices(source, plan, api),
  };
}

export function reconcileBuilderDecisions(decisions, api, ruleset) {
  const copy = clone(decisions || {});
  const plan = getBuilderPlan(copy, api, ruleset);
  const valid = new Set([
    ...plan.classChoices,
    ...plan.creationChoices,
    ...plan.creationAbilityChoices,
  ].map(choice => choice.id));
  const baseOf = key => String(key).replace(/#\d+$/, '').replace(/:(ability|feat|featability)$/, '');
  copy.featureChoices = { ...(copy.featureChoices || {}) };
  for (const key of Object.keys(copy.featureChoices)) {
    if (!valid.has(baseOf(key))) delete copy.featureChoices[key];
  }
  copy.abilityGrants = (Array.isArray(copy.abilityGrants) ? copy.abilityGrants : [])
    .filter(grant => valid.has(baseOf(grant?.id)));
  return copy;
}

export function applyBuilderChoice(decisions, change, api, ruleset) {
  const copy = clone(decisions || {});
  const plan = getBuilderPlan(copy, api, ruleset);
  const choiceId = String(change?.choiceId || '');
  if (!choiceId) return copy;
  copy.featureChoices = { ...(copy.featureChoices || {}) };
  copy.abilityGrants = Array.isArray(copy.abilityGrants) ? copy.abilityGrants.slice() : [];

  const abilityChoice = findAbilityChoice(plan, choiceId, copy, api);
  if (abilityChoice && change?.value && typeof change.value === 'object') {
    applyAbilityGrant(copy, abilityChoice, change.value);
    return copy;
  }

  const baseId = choiceId.replace(/:(feat|ability|featability)$/, '');
  const descriptor = plan.classChoices.concat(plan.creationChoices)
    .find(choice => choice.id === baseId || choice.id === choiceId);
  if (!descriptor) return copy;

  const value = change?.value == null ? '' : String(change.value);
  const key = choiceIdForSlot(choiceId, change?.slot, descriptor.count);
  setChoiceValue(copy.featureChoices, key, value);

  if (descriptor.kind !== 'asiMode') return copy;
  if (choiceId === descriptor.id) {
    if (value !== 'asi') removeGrant(copy, descriptor.ability.id);
    if (value !== 'feat') {
      delete copy.featureChoices[descriptor.feat.id];
      removeGrant(copy, descriptor.feat.ability.id);
    }
    return copy;
  }
  if (choiceId === descriptor.feat.id) {
    removeGrant(copy, descriptor.feat.ability.id);
    const feat = value ? api?.getItem?.('feat', value) : null;
    const increase = feat?.grants?.abilityScoreIncrease;
    const eligible = featAsiFrom(increase);
    if (increase && eligible.length === 1) {
      upsertGrant(copy, descriptor.feat.ability.id, { type: 'feat' }, {
        [eligible[0]]: Math.max(1, num(increase.amount, 1)),
      }, abilityCapFor(feat, descriptor));
    }
  }
  return copy;
}

function builderModel(source, api) {
  const baseStats = source.baseStats && Object.keys(source.baseStats).length
    ? { ...source.baseStats }
    : { ...(source.abilities || {}) };
  let classes = Array.isArray(source.classes) && source.classes.length
    ? source.classes.map(entry => ({ ...entry }))
    : null;
  if (!classes) {
    const record = source.className ? api?.getItemByName?.('class', source.className) : null;
    classes = source.className
      ? [{ classId: record?.id || '', level: Math.max(1, num(source.level, 1)), subclass: source.subclass || '' }]
      : [{ classId: '', level: 1, subclass: '' }];
  }
  return { baseStats, classes };
}

function collectClassChoices(classes, api, profile) {
  const out = [];
  const masteryEnabled = profile.capabilities.weaponMastery !== false;
  const advancement = profile.builder.abilityScoreAdvancement;
  for (const [classIndex, selected] of classes.entries()) {
    const record = selected.classId ? api?.getItem?.('class', selected.classId) : null;
    if (!record) continue;
    const classLevel = Math.max(1, num(selected.level, 1));
    const starting = record.startingProficiencies || {};
    const reduced = classIndex > 0 ? record.multiclassProficiencies : null;
    const skills = reduced?.skills || starting.skills;
    if (skills?.choose) out.push({
      id: `skills:${selected.classId}`,
      kind: 'skills',
      count: Math.max(1, num(skills.choose, 1)),
      from: Array.isArray(skills.from) ? skills.from.slice() : [],
      classId: selected.classId,
      source: { type: 'class', id: selected.classId, level: 1 },
    });
    appendRecordChoices(out, record.grants?.choices, {
      owner: selected.classId,
      classId: selected.classId,
      classLevel,
      sourceType: 'class',
      api,
      masteryEnabled,
    });
    const subclass = selected.subclass ? api?.getItem?.('subclass', selected.subclass) : null;
    appendRecordChoices(out, subclass?.grants?.choices, {
      owner: selected.subclass,
      classId: selected.classId,
      classLevel,
      sourceType: 'subclass',
      fallbackLevel: num(subclass?.subclassLevel, 3),
      api,
      masteryEnabled,
    });
    for (const feature of api?.listFeatures?.({ classId: selected.classId }) || []) {
      if (num(feature.level) > classLevel || (feature.subclassId && feature.subclassId !== selected.subclass)) continue;
      appendRecordChoices(out, feature.grants?.choices, {
        owner: feature.id,
        classId: selected.classId,
        classLevel,
        sourceType: 'feature',
        fallbackLevel: num(feature.level, 1),
        api,
        masteryEnabled,
      });
    }
    const levels = new Set(advancement.baseLevels);
    for (const level of Array.isArray(record.abilityScoreImprovementLevels)
      ? record.abilityScoreImprovementLevels
      : []) levels.add(num(level));
    for (const level of [...levels].filter(value => value > 0 && value <= classLevel).sort((a, b) => a - b)) {
      const id = `asi:${selected.classId}:${level}`;
      const categories = [...new Set([
        ...advancement.featCategories,
        ...(advancement.categoriesByLevel[level] || []),
      ])];
      out.push({
        id,
        kind: 'asiMode',
        classId: selected.classId,
        level,
        source: { type: 'class', id: selected.classId, level },
        ability: {
          id: `${id}:ability`, kind: 'abilityBudget', eligible: ABILITIES.slice(),
          budget: advancement.budget, perAbilityMax: advancement.perAbilityMax,
        },
        feat: {
          id: `${id}:feat`, categories,
          ability: {
            id: `${id}:featability`, kind: 'abilityBudget',
          },
          categoryAbilityCaps: { ...advancement.categoryAbilityCaps },
        },
      });
    }
  }
  const seen = new Set();
  return out.filter(choice => choice.id && !seen.has(choice.id) && seen.add(choice.id));
}

function appendRecordChoices(out, choices, context) {
  for (const choice of Array.isArray(choices) ? choices : []) {
    if (!choice?.id) continue;
    const sourceLevel = sourceLevelOf(choice, context.fallbackLevel || 1);
    if (sourceLevel > context.classLevel) continue;
    let from = Array.isArray(choice.from) ? choice.from.slice() : choice.from;
    if (!Array.isArray(from) && choice.fromCategory) {
      from = (context.api?.listFeatures?.({ category: choice.fromCategory }) || []).map(option => option.id);
    }
    const kind = choiceKind(choice, from);
    if (kind === 'weaponMastery' && !context.masteryEnabled) continue;
    let count = Math.max(1, num(choice.count, 1));
    let selectedLevel = -1;
    for (const [level, amount] of Object.entries(choice.countByLevel || {})) {
      const parsed = num(level);
      if (parsed <= context.classLevel && parsed > selectedLevel) {
        selectedLevel = parsed;
        count = Math.max(1, num(amount, count));
      }
    }
    out.push({
      id: choice.id,
      kind,
      count,
      from,
      category: choice.category,
      prompt: choice.prompt,
      default: choice.default,
      changeOn: choice.changeOn,
      classId: context.classId,
      source: { type: context.sourceType, id: context.owner, level: sourceLevel },
    });
  }
}

function collectCreationChoices(source, api) {
  const out = [];
  const append = (choice, owner, recordSource) => {
    if (!choice?.id) return;
    out.push({
      id: `${owner}:${choice.id}`,
      kind: choiceKind(choice, choice.from),
      count: Math.max(1, num(choice.count, 1)),
      from: Array.isArray(choice.from) ? choice.from.slice() : choice.from,
      category: choice.category,
      prompt: choice.prompt,
      default: choice.default,
      changeOn: choice.changeOn,
      source: recordSource,
    });
  };
  for (const origin of selectedOrigins(source, api)) {
    if (origin.record?.toolProficiencyChoice) {
      const choice = origin.record.toolProficiencyChoice;
      out.push({
        id: `${origin.type}:${origin.record.id}:tool`, kind: 'tools',
        count: Math.max(1, num(choice.count, 1)),
        from: Array.isArray(choice.from) ? choice.from.slice() : [],
        prompt: choice.prompt, source: origin.source,
      });
    }
    for (const choice of origin.record?.grants?.choices || []) {
      append(choice, `${origin.type}:${origin.record.id}`, origin.source);
    }
  }
  const featIds = selectedFeatIds(source, api);
  for (const featId of featIds) {
    const feat = api?.getItem?.('feat', featId);
    for (const choice of feat?.grants?.choices || []) {
      append(choice, `feat:${featId}`, { type: 'feat', id: featId, level: 1 });
    }
  }
  return out;
}

function collectCreationAbilityChoices(source, api, profile) {
  const configs = {
    background: profile.builder.backgroundAbilityGrant,
    species: profile.builder.speciesAbilityGrant,
  };
  const legacyIds = { background: 'bgasi', species: 'speciesasi' };
  const out = [];
  for (const origin of selectedOrigins(source, api)) {
    const config = configs[origin.type];
    const eligible = origin.record?.abilityScores;
    if (!config || !Array.isArray(eligible) || !eligible.length) continue;
    out.push({
      id: legacyIds[origin.type],
      kind: 'abilityBudget',
      eligible: eligible.slice(),
      budget: config.budget,
      perAbilityMax: config.perAbilityMax,
      source: origin.source,
    });
  }
  return out;
}

function selectedOrigins(source, api) {
  const origins = [];
  if (source.background) {
    const record = api?.getItemByName?.('background', source.background)
      || api?.getItem?.('background', source.background);
    if (record) origins.push({ type: 'background', record, source: { type: 'background', id: record.id, level: 1 } });
  }
  const speciesId = source.species || source.race;
  if (speciesId) {
    const record = api?.getItemByName?.('species', speciesId) || api?.getItem?.('species', speciesId);
    if (record) origins.push({ type: 'species', record, source: { type: 'species', id: record.id, level: 1 } });
  }
  return origins;
}

function selectedFeatIds(source, api) {
  const ids = new Set();
  for (const origin of selectedOrigins(source, api)) if (origin.record.originFeat) ids.add(origin.record.originFeat);
  for (const feat of Array.isArray(source.feats) ? source.feats : []) {
    const id = feat && (feat.featId || feat.id || feat);
    if (id) ids.add(id);
  }
  for (const feat of Array.isArray(source.extraFeats) ? source.extraFeats : []) if (feat?.featId) ids.add(feat.featId);
  for (const [key, value] of Object.entries(source.featureChoices || {})) if (key.endsWith(':feat') && value) ids.add(value);
  return ids;
}

function resolveChoices(source, plan, api) {
  const resolved = {
    skillProficiencies: [], toolProficiencies: [], skillExpertise: {}, feats: [],
    weaponMasteryChoices: [], languageProficiencies: [], saveProficiencies: [],
    weaponProficiencies: [], armorProficiencies: [], damageResistances: [], conditionImmunities: [],
  };
  for (const origin of selectedOrigins(source, api)) if (origin.record.originFeat) resolved.feats.push(origin.record.originFeat);
  for (const feat of Array.isArray(source.feats) ? source.feats : []) {
    const featId = feat && (feat.featId || feat.id || feat);
    if (featId) resolved.feats.push(featId);
  }
  for (const feat of Array.isArray(source.extraFeats) ? source.extraFeats : []) if (feat?.featId) resolved.feats.push(feat.featId);
  for (const choice of plan.classChoices.concat(plan.creationChoices)) {
    const values = choiceValues(source, choice);
    if (choice.kind === 'skills') resolved.skillProficiencies.push(...values);
    else if (choice.kind === 'tools') resolved.toolProficiencies.push(...values);
    else if (choice.kind === 'proficiencies') for (const value of values) {
      if (value.startsWith('skill:')) resolved.skillProficiencies.push(value.slice(6));
      else if (value.startsWith('tool:')) resolved.toolProficiencies.push(value.slice(5));
    }
    else if (choice.kind === 'expertise') for (const value of values) resolved.skillExpertise[value] = true;
    else if (choice.kind === 'skillExpertise') for (const value of values) {
      resolved.skillProficiencies.push(value);
      resolved.skillExpertise[value] = true;
    }
    else if (choice.kind === 'languages') resolved.languageProficiencies.push(...values);
    else if (choice.kind === 'savingThrows') resolved.saveProficiencies.push(...values);
    else if (choice.kind === 'weapons') resolved.weaponProficiencies.push(...values);
    else if (choice.kind === 'armor') resolved.armorProficiencies.push(...values);
    else if (choice.kind === 'resistances') resolved.damageResistances.push(...values);
    else if (choice.kind === 'immunities') resolved.conditionImmunities.push(...values);
    else if (choice.kind === 'weaponMastery') resolved.weaponMasteryChoices.push(...values);
    else if (choice.kind === 'feat') resolved.feats.push(...values);
    else if (choice.kind === 'asiMode' && source.featureChoices?.[choice.id] === 'feat') {
      const featId = source.featureChoices?.[choice.feat.id];
      if (featId) resolved.feats.push(featId);
    }
  }
  for (const key of Object.keys(resolved)) {
    if (Array.isArray(resolved[key])) resolved[key] = [...new Set(resolved[key])];
  }
  resolved.feats = resolved.feats.map(featId => ({ featId }));
  return resolved;
}

function choiceValues(source, choice) {
  const choices = source.featureChoices || {};
  const count = Math.max(1, num(choice.count, 1));
  const values = [];
  for (let index = 0; index < count; index++) {
    const key = count > 1 ? `${choice.id}#${index}` : choice.id;
    const value = choices[key] || (index === 0 ? choice.default : '');
    if (value && !values.includes(value)) values.push(value);
  }
  return values;
}

function choiceKind(choice, from) {
  const kinds = {
    skillProficiency: 'skills', expertise: 'expertise', skillExpertise: 'skillExpertise',
    toolProficiency: 'tools', proficiency: 'proficiencies', weaponMastery: 'weaponMastery',
    language: 'languages', savingThrowProficiency: 'savingThrows', weaponProficiency: 'weapons',
    armorProficiency: 'armor', damageResistance: 'resistances', conditionImmunity: 'immunities',
  };
  if (kinds[choice.type]) return kinds[choice.type];
  if (choice.type === 'feat' || (!Array.isArray(from) && choice.category)) return 'feat';
  return 'enumerated';
}

function sourceLevelOf(choice, fallback) {
  const parsed = num(String(choice.source || '').split(':')[1], fallback);
  return parsed > 0 ? parsed : fallback;
}

function findAbilityChoice(plan, id, source, api) {
  for (const choice of plan.creationAbilityChoices) if (choice.id === id) return choice;
  for (const choice of plan.classChoices) {
    if (choice.kind !== 'asiMode') continue;
    if (choice.ability.id === id) return { ...choice.ability, source: { type: 'asi' } };
    if (choice.feat.ability.id === id) {
      const featId = source.featureChoices?.[choice.feat.id];
      const feat = featId ? api?.getItem?.('feat', featId) : null;
      const increase = feat?.grants?.abilityScoreIncrease;
      const eligible = featAsiFrom(increase);
      const budget = Math.max(1, num(increase?.amount, 1));
      return {
        ...choice.feat.ability,
        eligible,
        budget,
        perAbilityMax: budget,
        cap: abilityCapFor(feat, choice),
        source: { type: 'feat' },
      };
    }
  }
  return null;
}

function applyAbilityGrant(copy, descriptor, value) {
  const ability = String(value.ability || '');
  if (!descriptor.eligible?.includes(ability) && descriptor.eligible) return;
  const grant = copy.abilityGrants.find(candidate => candidate?.id === descriptor.id);
  const assign = { ...(grant?.assign || {}) };
  const budget = Math.max(1, num(descriptor.budget, 1));
  const perAbilityMax = Math.max(1, num(descriptor.perAbilityMax, budget));
  const others = ABILITIES.reduce((total, key) => total + (key === ability ? 0 : Math.max(0, num(assign[key]))), 0);
  const amount = Math.max(0, Math.min(perAbilityMax, budget - others, num(value.amount)));
  if (amount) assign[ability] = amount; else delete assign[ability];
  upsertGrant(copy, descriptor.id, descriptor.source || { type: 'ability' }, assign, descriptor.cap);
}

function abilityCapFor(feat, descriptor) {
  const explicit = feat?.grants?.abilityScoreIncrease?.cap;
  if (explicit != null) return num(explicit);
  return descriptor.feat.categoryAbilityCaps[feat?.category] || null;
}

function choiceIdForSlot(id, slot, count) {
  const index = Math.max(0, num(slot));
  return Math.max(1, num(count, 1)) > 1 ? `${id}#${index}` : id;
}

function setChoiceValue(choices, key, value) {
  if (value) choices[key] = value;
  else delete choices[key];
}

function removeGrant(copy, id) {
  copy.abilityGrants = copy.abilityGrants.filter(grant => grant?.id !== id);
}

function upsertGrant(copy, id, source, assign, cap) {
  removeGrant(copy, id);
  if (!assign || !Object.keys(assign).length) return;
  copy.abilityGrants.push({ id, source, assign, ...(cap ? { cap: num(cap) } : {}) });
}

function clone(value) {
  if (typeof structuredClone === 'function') return structuredClone(value);
  return JSON.parse(JSON.stringify(value));
}
