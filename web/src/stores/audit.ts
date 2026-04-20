// SPDX-License-Identifier: GPL-2.0-only
import { createResource } from 'solid-js';
import { flowAPI } from '../client';

export interface AuditEvent {
  id: string;
  event_type: string;
  project_id?: string;
  workflow_id?: string;
  agent_id?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
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
    (f) => flowAPI.get<AuditEvent[]>(`/v1/audit${buildQuery(f)}`)
  );
  return { events, refetch };
}
