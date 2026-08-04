import { test } from 'node:test';
import assert from 'node:assert/strict';
import { dryRunRegister } from '../../ttrpg-codex/web/js/addon-test-harness.mjs';
import register from '../entry.js';
import { makeFake } from '../contract/synthetic-provider.mjs';

const META = {
  id: 'dnd-engine',
  name: 'D&D Rules Engine',
  version: '1.0.0',
  apiVersion: 2,
  hostVersion: '>=1.2.0',
  capabilities: { required: ['lifecycle.dispose'] },
  permissions: [],
  services: {
    provides: [{ contract: 'dnd5e.rules-engine', version: '1.0.0' }],
    consumes: [{ contract: 'dnd5e.rules-data', range: '^1.0.0', cardinality: 'one', required: false }],
  },
};

const dataHandle = (api = makeFake(), revision = 'fixture-1') => Object.freeze({
  api,
  provider: Object.freeze({
    addonId: 'unknown-rules-addon',
    addonName: 'Unknown Rules Addon',
    addonVersion: '7.0.0',
    contract: 'dnd5e.rules-data',
    contractVersion: '1.0.0',
    contentRevision: revision,
  }),
});

const engineApi = rec => rec.providedServices
  .find(service => service.contract === 'dnd5e.rules-engine')?.api;

test('headless addon publishes only the engine service and accepts an unknown provider id', () => {
  const { ok, rec, error } = dryRunRegister(register, META, {
    services: { 'dnd5e.rules-data': dataHandle() },
  });
  assert.ok(ok, error);
  assert.equal(rec.provided, undefined);
  assert.equal(rec.providedServices.length, 1);
  assert.equal(engineApi(rec).getContextIdentity().providerAddonId, 'unknown-rules-addon');
  for (const key of ['routes', 'sidebar', 'actions', 'fragmentOps', 'settingsTabs', 'collections']) {
    assert.equal(rec[key].length, 0, `${key} stays empty`);
  }
});

test('rules-data is optional and absence remains explicit', () => {
  const { ok, rec, error } = dryRunRegister(register, META);
  assert.ok(ok, error);
  assert.deepEqual(engineApi(rec).getAvailability(), {
    available: false,
    status: 'missing',
    errors: ['No rules-data service is selected.'],
  });
});

test('provider content revision is observable and replacement uses a fresh engine instance', () => {
  const first = dryRunRegister(register, META, { services: { 'dnd5e.rules-data': dataHandle(makeFake(), 'one') } });
  const second = dryRunRegister(register, META, { services: { 'dnd5e.rules-data': dataHandle(makeFake(), 'two') } });
  assert.equal(engineApi(first.rec).getContextIdentity().contentRevision, 'one');
  assert.equal(engineApi(second.rec).getContextIdentity().contentRevision, 'two');
});

test('stale service references fail clearly after disposal', async () => {
  const run = dryRunRegister(register, META, { services: { 'dnd5e.rules-data': dataHandle() } });
  const api = engineApi(run.rec);
  assert.equal(api.getAvailability().available, true);
  await run.dispose();
  assert.throws(() => api.getAvailability(), /disposed/);
  assert.throws(() => api.hydrate({ level: 1 }), /disposed/);
});
