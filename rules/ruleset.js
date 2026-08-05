// A validated rules profile is always supplied by the selected rules-data
// service. The engine deliberately has no native edition and never fills
// missing provider fields from a hidden default.

export function requireRuleset(record) {
  if (!record || typeof record !== 'object' || Array.isArray(record)) {
    throw new TypeError('A validated rules-data profile is required');
  }
  return record;
}
