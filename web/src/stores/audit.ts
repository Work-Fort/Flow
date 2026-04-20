// SPDX-License-Identifier: GPL-2.0-only
import { createResource } from 'solid-js';
import { flowAPI } from '../client';

export interface AuditEvent {
  id: string;
  occurred_at: string;
  type: string;
  agent_id: string;
  agent_name?: string;
  workflow_id?: string;
  role?: string;
  project?: string;
}

interface AuditListResponse {
  events: AuditEvent[];
}

export interface AuditFilter {
  project_id?: string;
  workflow_id?: string;
  agent_id?: string;
  event_type?: string;
  since?: string;
  until?: string;
  limit?: number;
  offset?: number;
}

function buildQuery(filter: AuditFilter): string {
  const params = new URLSearchParams();
  if (filter.project_id) params.set('project_id', filter.project_id);
  if (filter.workflow_id) params.set('workflow_id', filter.workflow_id);
  if (filter.agent_id) params.set('agent_id', filter.agent_id);
  if (filter.event_type) params.set('event_type', filter.event_type);
  if (filter.since) params.set('since', filter.since);
  if (filter.until) params.set('until', filter.until);
  if (filter.limit) params.set('limit', String(filter.limit));
  if (filter.offset) params.set('offset', String(filter.offset));
  const q = params.toString();
  return q ? `?${q}` : '';
}

export function createAuditStore(filter: () => AuditFilter = () => ({})) {
  const [events, { refetch }] = createResource(
    filter,
    async (f): Promise<AuditEvent[]> => {
      const res = await flowAPI.get<AuditListResponse>(`/v1/audit${buildQuery(f)}`);
      return res.events ?? [];
    }
  );
  return { events, refetch };
}
