// SPDX-License-Identifier: GPL-2.0-only
import { createResource } from 'solid-js';
import { flowAPI } from '../client';

export interface WorkItem {
  id: string;
  title?: string;
  status?: string;
  assigned_agent?: string;
  priority?: number;
  instance_id: string;
  updated_at: string;
  created_at: string;
}

export interface WorkItemHistoryEntry {
  step_name: string;
  agent_id?: string;
  transitioned_at: string;
}

export function createWorkItemsStore(projectId: () => string | null) {
  const [workItems, { refetch }] = createResource(
    () => projectId() ?? undefined,
    (id) => flowAPI.get<WorkItem[]>(`/v1/projects/${id}/work-items`)
  );
  return { workItems, refetch };
}

export function createWorkItemHistoryStore(itemId: () => string | null) {
  const [history] = createResource(
    () => itemId() ?? undefined,
    (id) => flowAPI.get<WorkItemHistoryEntry[]>(`/v1/items/${id}/history`)
  );
  return { history };
}
