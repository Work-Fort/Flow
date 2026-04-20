// SPDX-License-Identifier: GPL-2.0-only
import { For, Show } from 'solid-js';
import { createWorkItemsStore, type WorkItem } from '../stores/work-items';

interface Props {
  projectId: string;
  onSelect: (item: WorkItem) => void;
}

export function WorkItemList(props: Props) {
  const { workItems } = createWorkItemsStore(() => props.projectId);

  return (
    <div>
      <Show when={workItems.loading}>
        <wf-skeleton />
      </Show>
      <Show when={workItems.error}>
        <wf-banner variant="error" headline="Failed to load work items" />
      </Show>
      <Show when={workItems()}>
        <wf-list>
          <For each={workItems()!}>
            {(item) => (
              <wf-list-item on:wf-select={() => props.onSelect(item)}>
                <span>{item.title || item.id}</span>
                <Show when={item.current_step_id}>
                  <wf-badge
                    data-wf="trailing"
                    count={undefined}
                    style="margin-left: var(--wf-space-sm);"
                  >
                    {item.current_step_id}
                  </wf-badge>
                </Show>
                <Show when={item.assigned_agent_id}>
                  <span style="color: var(--wf-color-text-muted); font-size: var(--wf-text-sm);">
                    {item.assigned_agent_id}
                  </span>
                </Show>
              </wf-list-item>
            )}
          </For>
        </wf-list>
        <Show when={(workItems() ?? []).length === 0}>
          <p style="color: var(--wf-color-text-muted); padding: var(--wf-space-md);">
            No work items for this project.
          </p>
        </Show>
      </Show>
    </div>
  );
}
