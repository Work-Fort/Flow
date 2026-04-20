// SPDX-License-Identifier: GPL-2.0-only
import { For, Show } from 'solid-js';
import { createWorkItemHistoryStore, type WorkItem } from '../stores/work-items';

interface Props {
  item: WorkItem;
  onBack: () => void;
}

export function WorkItemDetail(props: Props) {
  const { history } = createWorkItemHistoryStore(() => props.item.id);

  return (
    <div>
      <wf-button variant="text" on:click={props.onBack}>← Work Items</wf-button>
      <h2>{props.item.title ?? props.item.id}</h2>

      <dl style="display: grid; grid-template-columns: max-content 1fr; gap: var(--wf-space-sm) var(--wf-space-md); margin-bottom: var(--wf-space-md);">
        <dt style="color: var(--wf-color-text-muted);">Status</dt>
        <dd>{props.item.status ?? '—'}</dd>
        <dt style="color: var(--wf-color-text-muted);">Assigned</dt>
        <dd>{props.item.assigned_agent ?? '—'}</dd>
        <dt style="color: var(--wf-color-text-muted);">Priority</dt>
        <dd>{props.item.priority ?? '—'}</dd>
        <dt style="color: var(--wf-color-text-muted);">Updated</dt>
        <dd>{new Date(props.item.updated_at).toLocaleString()}</dd>
      </dl>

      <h3>Transition History</h3>
      <Show when={history.loading}><wf-skeleton /></Show>
      <Show when={history()}>
        <wf-list>
          <For each={history()!}>
            {(entry) => (
              <wf-list-item>
                <span>{entry.step_name}</span>
                <Show when={entry.agent_id}>
                  <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                    {entry.agent_id}
                  </span>
                </Show>
                <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                  {new Date(entry.transitioned_at).toLocaleString()}
                </span>
              </wf-list-item>
            )}
          </For>
        </wf-list>
        <Show when={(history() ?? []).length === 0}>
          <p style="color: var(--wf-color-text-muted);">No history yet.</p>
        </Show>
      </Show>
    </div>
  );
}
