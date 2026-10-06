import type { ManagedConnection } from '../api/schema'
export const emptyConnection: ManagedConnection = {
  id: '',
  name: '',
  region: '',
  authentication: 'local',
  server: '',
  credentialConfigured: false,
  disabled: true,
  namespaces: [{ name: '', team: '', environment: 'production', location: '' }],
  operators: [],
}
