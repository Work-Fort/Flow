// SPDX-License-Identifier: GPL-2.0-only
import { createSignal, For, Show } from 'solid-js';
import { createAuditStore, type AuditFilter } from '../stores/audit';
import type { Project } from '../stores/projects';

interface Props {
  project?: Project | null;
}

export function AuditLogView(props: Props) {
  const [filterWorkflow, setFilterWorkflow] = createSignal('');
  const [filterAgent, setFilterAgent] = createSignal('');
  const [filterType, setFilterType] = createSignal('');

  const filter = (): AuditFilter => ({
    project_id: props.project?.id,
    workflow_id: filterWorkflow() || undefined,
    agent_id: filterAgent() || undefined,
    event_type: filterType() || undefined,
    limit: 100,
  });

  const { events, refetch } = createAuditStore(filter);

  return (
    <div>
      <h2>Audit Log{props.project ? ` — ${props.project.name}` : ''}</h2>

      <div style="display: flex; gap: var(--wf-space-sm); margin-bottom: var(--wf-space-md); flex-wrap: wrap;">
        <wf-input
          placeholder="Filter by workflow ID"
          value={filterWorkflow()}
          on:input={(e: Event) => setFilterWorkflow((e.target as HTMLInputElement).value)}
        />
        <wf-input
          placeholder="Filter by agent ID"
          value={filterAgent()}
          on:input={(e: Event) => setFilterAgent((e.target as HTMLInputElement).value)}
        />
        <wf-input
          placeholder="Filter by event type"
          value={filterType()}
          on:input={(e: Event) => setFilterType((e.target as HTMLInputElement).value)}
        />
        <wf-button variant="text" on:click={() => refetch()}>Refresh</wf-button>
      </div>

      <Show when={events.loading && !events()}><wf-skeleton /></Show>
      <Show when={events.error}>
        <wf-banner variant="error" headline="Failed to load audit events" />
      </Show>
      <Show when={events()}>
        <wf-list>
          <For each={events()!}>
            {(event) => (
              <wf-list-item>
                <span style="font-size: var(--wf-text-sm); color: var(--wf-color-text-muted);">
                  {new Date(event.created_at).toLocaleString()}
                </span>
                <wf-badge data-wf="trailing" style="margin: 0 var(--wf-space-sm);">
                  {event.event_type}
                </wf-badge>
                <Show when={event.project_id}>
                  <span style="color: var(--wf-color-text-secondary); font-size: var(--wf-text-sm);">
                    {event.project_id}
                  </span>
                </Show>
                <Show when={event.agent_id}>
                  <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                    {event.agent_id}
                  </span>
                </Show>
              </wf-list-item>
            )}
          </For>
        </wf-list>
        <Show when={(events() ?? []).length === 0}>
          <p style="color: var(--wf-color-text-muted); padding: var(--wf-space-md);">No events found.</p>
        </Show>
      </Show>
    </div>
  );
}
