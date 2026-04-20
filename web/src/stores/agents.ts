// SPDX-License-Identifier: GPL-2.0-only
import { createResource, onCleanup } from 'solid-js';
import { flowAPI } from '../client';

export interface AgentEntry {
  id: string;
  name?: string;
  model?: string;
  runtime?: string;
  assigned?: boolean;
  project_id?: string;
  role?: string;
  workflow_id?: string;
  lease_expires_at?: string;
}

export interface AgentFilter {
  assigned?: boolean;
  role?: string;
  project_id?: string;
}

function buildQuery(filter: AgentFilter): string {
  const params = new URLSearchParams();
  if (filter.assigned !== undefined) params.set('assigned', String(filter.assigned));
  if (filter.role) params.set('role', filter.role);
  if (filter.project_id) params.set('project_id', filter.project_id);
  const q = params.toString();
  return q ? `?${q}` : '';
}

export function createAgentsStore(filter: () => AgentFilter = () => ({}), pollMs = 5000) {
  const [agents, { refetch }] = createResource(
    filter,
    (f) => flowAPI.get<AgentEntry[]>(`/v1/agents${buildQuery(f)}`)
  );

  const interval = setInterval(() => refetch(), pollMs);
  onCleanup(() => clearInterval(interval));

  return { agents, refetch };
}
