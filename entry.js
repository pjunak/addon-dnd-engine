import {
  RULES_ENGINE_CONTRACT,
  RULES_ENGINE_CONTRACT_VERSION,
  makeRulesApi,
} from './rules/api.js';

export default function register(host) {
  let disposed = false;
  const api = makeRulesApi(
    () => host.useService('dnd5e.rules-data'),
    { isDisposed: () => disposed },
  );
  host.provideService(RULES_ENGINE_CONTRACT, RULES_ENGINE_CONTRACT_VERSION, api);
  return () => { disposed = true; };
}
