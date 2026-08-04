const list = (value) => Array.isArray(value) ? value : [];

export const featIdOf = (feat) => feat && (feat.featId || feat.id || feat);

export function selectedFeatRecords(decisions, api) {
  return list(decisions && decisions.feats)
    .map(featIdOf)
    .filter(Boolean)
    .map((id) => ({ id, record: api && api.getItem ? api.getItem('feat', id) : null }))
    .filter(({ record }) => record);
}

export function collectGrantSources({
  classes,
  species,
  lineage,
  background,
  featRecords,
  api,
  totalLevel,
}) {
  const sources = [];
  const add = (record, source, level) => {
    if (!record) return;
    sources.push({
      record,
      grants: record.grants || {},
      source: { ...source, level },
      level,
    });
  };

  for (const current of list(classes)) {
    add(current.record, { type: 'class', id: current.classId }, current.level);
    const subclass = current.subclass && api && api.getItem
      ? api.getItem('subclass', current.subclass)
      : null;
    add(subclass, { type: 'subclass', id: current.subclass }, current.level);

    if (!api || !api.listFeatures) continue;
    for (const feature of api.listFeatures({ classId: current.classId })) {
      if (!feature || Number(feature.level) > current.level) continue;
      if (feature.subclassId && feature.subclassId !== current.subclass) continue;
      const record = api.getItem ? (api.getItem('feature', feature.id) || feature) : feature;
      add(record, { type: 'feature', id: feature.id }, current.level);
    }
  }

  add(species, { type: 'species', id: species && species.id }, totalLevel);
  if (lineage) {
    add(lineage, {
      type: 'species',
      id: species && species.id,
    }, totalLevel);
  }
  add(background, { type: 'background', id: background && background.id }, totalLevel);
  for (const feat of list(featRecords)) {
    add(feat.record, { type: 'feat', id: feat.id }, totalLevel);
  }
  return sources;
}

export function applyChoicePackages(sources, featureChoices) {
  const selected = featureChoices || {};
  return list(sources).map((source) => {
    let grants = source.grants || {};
    for (const declaration of list(grants.choicePackages)) {
      if (!declaration || !declaration.choiceId) continue;
      const scopedId = `${source.source.type}:${source.source.id}:${declaration.choiceId}`;
      const selectedValue = selected[scopedId] ?? selected[declaration.choiceId];
      const option = declaration.options
        ? declaration.options[selectedValue]
        : null;
      if (!option) continue;
      const merged = { ...grants };
      for (const [field, value] of Object.entries(option)) {
        if (Array.isArray(value)) merged[field] = list(merged[field]).concat(value);
        else merged[field] = value;
      }
      grants = merged;
    }
    return { ...source, grants };
  });
}

export function activationKey(source, activation) {
  return `${source.source.type}:${source.source.id}:${activation.id}`;
}


