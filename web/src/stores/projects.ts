// SPDX-License-Identifier: GPL-2.0-only
import { createResource } from 'solid-js';
import { flowAPI } from '../client';

export interface Project {
  id: string;
  name: string;
  description?: string;
  template_id?: string;
  channel_name?: string;
  vocabulary_id?: string;
  retention_days?: number | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectBody {
  name: string;
  description?: string;
  template_id?: string;
  channel_name?: string;
  vocabulary_id?: string;
  retention_days?: number | null;
  channel_already_exists?: boolean;
}

export interface PatchProjectBody {
  name?: string;
  description?: string;
  retention_days?: number | null;
  clear_retention_days?: boolean;
}

export function createProjectsStore() {
  const [projects, { refetch: reload }] = createResource<Project[]>(
    () => flowAPI.get<Project[]>('/v1/projects')
  );

  async function createProject(body: CreateProjectBody): Promise<Project> {
    const p = await flowAPI.post<Project>('/v1/projects', body);
    reload();
    return p;
  }

  async function patchProject(id: string, body: PatchProjectBody): Promise<Project> {
    const p = await flowAPI.patch<Project>(`/v1/projects/${id}`, body);
    reload();
    return p;
  }

  async function deleteProject(id: string): Promise<void> {
    await flowAPI.delete(`/v1/projects/${id}`);
    reload();
  }

  return { projects, reload, createProject, patchProject, deleteProject };
}
