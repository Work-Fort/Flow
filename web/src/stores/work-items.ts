// SPDX-License-Identifier: GPL-2.0-only
import { createResource } from 'solid-js';
import { flowAPI } from '../client';

export interface WorkItem {
  id: string;
  instance_id: string;
  title: string;
  description: string;
  current_step_id: string;
  assigned_agent_id?: string;
  priority: string;
  created_at: string;
  updated_at: string;
}

export interface WorkItemHistoryEntry {
  id: string;
  work_item_id: string;
  from_step_id: string;
  to_step_id: string;
  transition_id: string;
  triggered_by: string;
  reason?: string;
  timestamp: string;
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
