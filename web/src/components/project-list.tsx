// SPDX-License-Identifier: GPL-2.0-only
import { createSignal, For, Show } from 'solid-js';
import { createResource } from 'solid-js';
import { flowAPI } from '../client';
import type { Project } from '../stores/projects';

interface Props {
  onSelect: (project: Project) => void;
}

export function ProjectList(props: Props) {
  const [projects, { refetch }] = createResource<Project[]>(
    () => flowAPI.get<Project[]>('/v1/projects')
  );

  return (
    <div>
      <Show when={projects.loading}>
        <wf-skeleton />
      </Show>
      <Show when={projects.error}>
        <wf-banner variant="error" headline="Failed to load projects" />
      </Show>
      <Show when={projects()}>
        <wf-list>
          <For each={projects()!}>
            {(p) => (
              <wf-list-item on:wf-select={() => props.onSelect(p)}>
                <span>{p.name}</span>
                <Show when={p.channel_name}>
                  <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                    {p.channel_name}
                  </span>
                </Show>
              </wf-list-item>
            )}
          </For>
        </wf-list>
        <Show when={(projects() ?? []).length === 0}>
          <p style="color: var(--wf-color-text-muted); padding: var(--wf-space-md);">
            No projects yet.
          </p>
        </Show>
      </Show>
    </div>
  );
}
