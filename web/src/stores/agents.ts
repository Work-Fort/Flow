// SPDX-License-Identifier: GPL-2.0-only
import { createResource, onCleanup } from 'solid-js';
import { flowAPI } from '../client';

export interface AgentEntry {
  id: string;
  name: string;
  team_id?: string;
  model?: string;
  runtime?: string;
  current_role?: string;
  current_project?: string;
  current_workflow_id?: string;
  lease_expires_at?: string | null;
  // derived — not on the wire; computed in store
  assigned: boolean;
}

export interface AgentFilter {
  assigned?: boolean;
  role?: string;
  project?: string;
}

interface AgentListResponse {
  agents: Omit<AgentEntry, 'assigned'>[];
}

function buildQuery(filter: AgentFilter): string {
  const params = new URLSearchParams();
  if (filter.assigned !== undefined) params.set('assigned', String(filter.assigned));
  if (filter.role) params.set('role', filter.role);
  if (filter.project) params.set('project', filter.project);
  const q = params.toString();
  return q ? `?${q}` : '';
}

export function createAgentsStore(filter: () => AgentFilter = () => ({}), pollMs = 5000) {
  const [agents, { refetch }] = createResource(
    filter,
    async (f): Promise<AgentEntry[]> => {
      const res = await flowAPI.get<AgentListResponse>(`/v1/agents${buildQuery(f)}`);
      return (res.agents ?? []).map(a => ({
        ...a,
        assigned: a.lease_expires_at != null,
      }));
    }
  );

  const interval = setInterval(() => refetch(), pollMs);
  onCleanup(() => clearInterval(interval));

  return { agents, refetch };
}
