// SPDX-License-Identifier: GPL-2.0-only
import { createSignal, For, Show } from 'solid-js';
import { createAgentsStore, type AgentFilter } from '../stores/agents';

export function AgentPoolView() {
  const [filterAssigned, setFilterAssigned] = createSignal<boolean | undefined>(undefined);
  const [filterRole, setFilterRole] = createSignal('');
  const [filterProject, setFilterProject] = createSignal('');

  const filter = (): AgentFilter => ({
    assigned: filterAssigned(),
    role: filterRole() || undefined,
    project_id: filterProject() || undefined,
  });

  const { agents } = createAgentsStore(filter);

  function leaseCountdown(expiresAt: string | undefined): string {
    if (!expiresAt) return '—';
    const diff = new Date(expiresAt).getTime() - Date.now();
    if (diff <= 0) return 'expired';
    const s = Math.floor(diff / 1000);
    const m = Math.floor(s / 60);
    return m > 0 ? `${m}m ${s % 60}s` : `${s}s`;
  }

  return (
    <div>
      <h2>Agent Pool</h2>

      <div style="display: flex; gap: var(--wf-space-sm); margin-bottom: var(--wf-space-md); flex-wrap: wrap;">
        <wf-select
          value={filterAssigned() === undefined ? '' : String(filterAssigned())}
          on:change={(e: Event) => {
            const v = (e.target as HTMLSelectElement).value;
            setFilterAssigned(v === '' ? undefined : v === 'true');
          }}
        >
          <option value="">Any assignment</option>
          <option value="true">Claimed</option>
          <option value="false">Idle</option>
        </wf-select>
        <wf-input
          placeholder="Filter by role"
          value={filterRole()}
          on:input={(e: Event) => setFilterRole((e.target as HTMLInputElement).value)}
        />
        <wf-input
          placeholder="Filter by project ID"
          value={filterProject()}
          on:input={(e: Event) => setFilterProject((e.target as HTMLInputElement).value)}
        />
      </div>

      <Show when={agents.loading && !agents()}><wf-skeleton /></Show>
      <Show when={agents.error}>
        <wf-banner variant="error" headline="Failed to load agents" />
      </Show>
      <Show when={agents()}>
        <wf-list>
          <For each={agents()!}>
            {(agent) => (
              <wf-list-item>
                <span style="font-weight: bold;">{agent.name ?? agent.id}</span>
                <wf-badge data-wf="trailing" style="margin-left: var(--wf-space-sm);">
                  {agent.assigned ? 'Claimed' : 'Idle'}
                </wf-badge>
                <Show when={agent.assigned}>
                  <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                    {agent.project_id} · {agent.role} · {leaseCountdown(agent.lease_expires_at)}
                  </span>
                </Show>
                <Show when={agent.model}>
                  <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                    {agent.model}
                  </span>
                </Show>
              </wf-list-item>
            )}
          </For>
        </wf-list>
        <Show when={(agents() ?? []).length === 0}>
          <p style="color: var(--wf-color-text-muted); padding: var(--wf-space-md);">No agents found.</p>
        </Show>
      </Show>
    </div>
  );
}
