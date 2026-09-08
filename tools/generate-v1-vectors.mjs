// Reproduce reviewed vectors from repository-owned synthetic v1 fixtures only.
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, dirname, relative, isAbsolute } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';

const root = fileURLToPath(new URL('../', import.meta.url));
const revision = 'b2ba2be2940d63fe6c7d769772773540748a6355';
const scratch = mkdtempSync(resolve(tmpdir(), 'codex-engine-reference-'));
try {
  const files = ['rules/api.js', 'rules/builder.js', 'rules/engine.js', 'rules/grants.js', 'rules/ruleset.js',
    'contract/rules-data.js', 'contract/synthetic-provider.mjs', 'contract/synthetic-rulesets.mjs', 'tests/engine.mjs'];
  const hashes = {};
  for (const path of files) {
    const source = execFileSync('git', ['-c', `safe.directory=${root.replaceAll('\\', '/')}`, 'show', `${revision}:${path}`], { cwd: root, windowsHide: true });
    const target = resolve(scratch, path); mkdirSync(dirname(target), { recursive: true }); writeFileSync(target, source);
    hashes[path] = createHash('sha256').update(source).digest('hex');
  }
  writeFileSync(resolve(scratch, 'package.json'), '{"type":"module"}');
  execFileSync(process.execPath, ['--test', 'tests/engine.mjs'], { cwd: scratch, windowsHide: true, stdio: 'inherit' });
  const { makeFake } = await import(pathToFileURL(resolve(scratch, 'contract/synthetic-provider.mjs')));
  const { makeRulesApi } = await import(pathToFileURL(resolve(scratch, 'rules/api.js')));
  const profiles = await import(pathToFileURL(resolve(scratch, 'contract/synthetic-rulesets.mjs')));
  const rulesets = { '2024': profiles.SYNTHETIC_2024_RULESET, '2014': profiles.SYNTHETIC_2014_RULESET };
  const data = makeFake(), records = {};
  for (const kind of ['class','subclass','species','background','feat','feature','spell','weapon','armor','skill','magic-item']) records[kind] = data.getRecords(kind);
  const apiFor = edition => makeRulesApi(() => edition ? { api: { ...data, getRuleset: () => rulesets[edition] },
    provider: { addonId: 'fixture-provider', addonName: 'Fixture Provider', addonVersion: '2.3.4', contract: 'dnd5e.rules-data', contractVersion: '2.0.0', contentRevision: 'content-a' } } : null);
  const cases = [], clean = value => JSON.parse(JSON.stringify(value));
  const add = (name, operation, input, edition = '2024') => {
    const api = apiFor(edition), args = clean(input);
    const expected = operation === 'hydrate' ? api.hydrate(args) : operation === 'builder-plan' ? api.getBuilderPlan(args) :
      operation === 'reconcile' ? api.reconcileBuilderDecisions(args) : api.applyBuilderChoice(args.decisions, args.change);
    const vector = { name, edition, operation, input: clean(input), expected: clean(expected) };
    // Reviewed Go correction: v1 used these class grants for weapon attacks but
    // omitted them from the displayed/saved proficiency summary. Keep the original
    // result intact and assert the precise corrected field separately in Go.
    if (operation === 'hydrate' && edition) {
      const classGrants = { fighter: ['simple', 'martial'], rogue: ['martial-finesse-or-light'] };
      const tokens = [...new Set((input.classes || []).flatMap(cls => classGrants[cls.classId] || []))];
      if (tokens.length) vector.classWeaponProficiencies = tokens;
    }
    cases.push(vector);
    return clean(expected);
  };
  const abilities = { STR: 16, DEX: 14, CON: 14, INT: 16, WIS: 12, CHA: 13 };
  for (const edition of ['2014', '2024']) {
    for (const cls of records.class) for (const level of [1, 3, 5, 11, 19]) {
      const decisions = { abilities, classes: [{ classId: cls.id, level }] };
      add(`${edition}-${cls.id}-${level}`, 'hydrate', decisions, edition);
      if (level === 19) add(`${edition}-${cls.id}-plan`, 'builder-plan', { ...decisions, background: 'Acolyte', species: 'Dwarf' }, edition);
    }
    for (const classes of [[{classId:'wizard',level:3},{classId:'fighter',level:1}], [{classId:'fighter',level:1},{classId:'wizard',level:3}],
      [{classId:'paladin',level:5},{classId:'sorcerer',level:1}], [{classId:'warlock',level:5},{classId:'wizard',level:3}],
      [{classId:'fighter',level:7,subclass:'eldritch-knight'},{classId:'wizard',level:1}]]) {
      add(`${edition}-multi-${classes.map(cls=>`${cls.classId}${cls.level}`).join('-')}`, 'hydrate', { abilities, classes }, edition);
    }
  }
  for (const [name, extras] of Object.entries({
    'dwarf-lineage': { species: 'Dwarf', lineage: 'hill-dwarf', feats: ['tough'] },
    'drow-spells': { species: 'Elf', lineage: 'drow', background: 'Acolyte' },
    'wood-speed': { species: 'Elf', lineage: 'wood-elf' },
    'equipped': { inventory: [{ ref: 'longsword', location: 'equipped' }, { ref: 'breastplate', location: 'equipped' }] },
    'finesse': { inventory: [{ ref: 'dagger', location: 'equipped' }, { ref: 'rapier', location: 'equipped' }], skills: { stealth: 'expertise' } },
    'malformed-armor': { inventory: [{ ref: 'brokenplate', location: 'equipped' }] },
    'missing-references': { species: 'Unknown species', background: 'Unknown background', feats: ['unknown-feat'] },
    'play-state': { hp: { current: 7, max: 80, temp: 2 }, resources: [{ name: 'Manual', current: 3, max: 4 }], notes: 'Authored', currency: { gp: 9 } },
  })) add(name, 'hydrate', { abilities, classes: [{ classId: 'barbarian', level: 5 }], ...extras });
  for (const feat of records.feat) add(`feat-${feat.id}`, 'hydrate', {
    abilities, classes: [{ classId: 'wizard', level: 5 }], feats: [{ featId: feat.id }],
    grantChoices: { 'feat:magic-initiate:mi-cantrips': ['fire-bolt'], 'feat:magic-initiate:mi-spell': ['mage-armor'] },
  });
  for (const edition of ['2014', '2024']) {
    add(`${edition}-rogue-choices`, 'hydrate', { abilities, classes: [{ classId: 'rogue', level: 6 }],
      featureChoices: { 'skills:rogue': ['stealth', 'perception'], 'rogue-expertise-1': ['stealth', 'perception'] },
      inventory: [{ ref: 'rapier', location: 'equipped' }], weaponMasteryChoices: ['rapier'] }, edition);
    add(`${edition}-ability-caps`, 'hydrate', { baseStats: { STR: 19, CON: 29 }, classes: [{ classId: 'wizard', level: 5 }],
      abilityGrants: [{ id: 'manual', source: { type: 'manual' }, assign: { STR: 4, CON: 2 }, cap: 30 }] }, edition);
  }
  add('missing-provider', 'hydrate', { abilities, level: 5 }, '');
  let decisions = { background: 'Acolyte', classes: [{ classId: 'fighter', level: 19 }], featureChoices: {}, abilityGrants: [] };
  const changes = [
    { choiceId:'bgasi', value:{ability:'INT',amount:3} }, { choiceId:'asi:fighter:4',value:'asi' },
    { choiceId:'asi:fighter:4:ability',value:{ability:'STR',amount:2} }, { choiceId:'asi:fighter:19',value:'feat' },
    { choiceId:'asi:fighter:19:feat',value:'boon-of-fortitude' }, { choiceId:'asi:fighter:19:feat',value:'boon-of-fate' },
    { choiceId:'asi:fighter:19:featability',value:{ability:'WIS',amount:1} }, { choiceId:'asi:fighter:19',value:'asi' },
  ];
  for (const [index, change] of changes.entries()) {
    decisions = add(`builder-change-${index}`, 'apply', { decisions, change });
    add(`builder-hydrate-${index}`, 'hydrate', decisions);
  }
  add('reconcile-level-loss', 'reconcile', { ...decisions, classes:[{classId:'fighter',level:1}] });
  const result = { reference: { revision, files: hashes }, rulesets, records, cases };
  // One case per line keeps complete structured results reviewable without enormous diffs.
  const json = `{"reference":${JSON.stringify(result.reference)},\n"rulesets":${JSON.stringify(rulesets)},\n"records":${JSON.stringify(records)},\n"cases":[\n${cases.map(item=>JSON.stringify(item)).join(',\n')}\n]}\n`;
  writeFileSync(resolve(root, 'testdata/v1-parity.json'), json);
  console.log(`Wrote ${cases.length} v1 parity vectors from ${revision}`);
} finally {
  const child = relative(resolve(tmpdir()), scratch);
  if (!child || child.startsWith('..') || isAbsolute(child)) throw Error('Unsafe reference cleanup path');
  rmSync(scratch, { recursive: true, force: true });
}
