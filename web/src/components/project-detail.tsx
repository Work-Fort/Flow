// SPDX-License-Identifier: GPL-2.0-only
import { createSignal, Show } from 'solid-js';
import type { Project, PatchProjectBody } from '../stores/projects';
import { flowAPI } from '../client';
import { BotPanel } from './bot-panel';

interface Props {
  project: Project;
  onUpdated: (p: Project) => void;
  onDeleted: () => void;
}

const RETENTION_OPTIONS = [
  { label: 'Permanent', value: '' },
  { label: '30 days', value: '30' },
  { label: '90 days', value: '90' },
  { label: '180 days', value: '180' },
  { label: '365 days', value: '365' },
];

export function ProjectDetail(props: Props) {
  const [editing, setEditing] = createSignal(false);
  const [confirmDelete, setConfirmDelete] = createSignal(false);
  const [name, setName] = createSignal(props.project.name);
  const [description, setDescription] = createSignal(props.project.description ?? '');
  const [retentionDays, setRetentionDays] = createSignal(
    props.project.retention_days != null ? String(props.project.retention_days) : ''
  );

  let editDialogRef!: HTMLElement & { show(): void; hide(): void };
  let deleteDialogRef!: HTMLElement & { show(): void; hide(): void };

  function openEdit() {
    setName(props.project.name);
    setDescription(props.project.description ?? '');
    setRetentionDays(props.project.retention_days != null ? String(props.project.retention_days) : '');
    setEditing(true);
    editDialogRef?.show?.();
  }

  async function handleSave() {
    const body: PatchProjectBody = {
      name: name().trim() || undefined,
      description: description().trim() || undefined,
    };
    if (retentionDays()) {
      body.retention_days = parseInt(retentionDays());
    } else {
      body.clear_retention_days = true;
    }
    const updated = await flowAPI.patch<Project>(`/v1/projects/${props.project.id}`, body);
    setEditing(false);
    editDialogRef?.hide?.();
    props.onUpdated(updated);
  }

  async function handleDelete() {
    await flowAPI.delete(`/v1/projects/${props.project.id}`);
    deleteDialogRef?.hide?.();
    props.onDeleted();
  }

  return (
    <div>
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--wf-space-md);">
        <h2 style="margin: 0;">{props.project.name}</h2>
        <div style="display: flex; gap: var(--wf-space-sm);">
          <wf-button variant="text" on:click={openEdit}>Edit</wf-button>
          <wf-button variant="text" on:click={() => { setConfirmDelete(true); deleteDialogRef?.show?.(); }}>
            Delete
          </wf-button>
        </div>
      </div>

      <Show when={props.project.description}>
        <p style="color: var(--wf-color-text-secondary);">{props.project.description}</p>
      </Show>

      <dl style="display: grid; grid-template-columns: max-content 1fr; gap: var(--wf-space-sm) var(--wf-space-md);">
        <dt style="color: var(--wf-color-text-muted);">Channel</dt>
        <dd>{props.project.channel_name ?? '—'}</dd>
        <dt style="color: var(--wf-color-text-muted);">Vocabulary</dt>
        <dd>{props.project.vocabulary_id ?? '—'}</dd>
        <dt style="color: var(--wf-color-text-muted);">Retention</dt>
        <dd>{props.project.retention_days != null ? `${props.project.retention_days} days` : 'Permanent'}</dd>
      </dl>

      <BotPanel projectId={props.project.id} />

      {/* Edit dialog */}
      <wf-dialog ref={editDialogRef} header="Edit Project" on:wf-close={() => setEditing(false)}>
        <div style="display: flex; flex-direction: column; gap: var(--wf-space-md); padding: var(--wf-space-sm);">
          <wf-input
            placeholder="Project name"
            value={name()}
            on:input={(e: Event) => setName((e.target as HTMLInputElement).value)}
          />
          <wf-input
            placeholder="Description"
            value={description()}
            on:input={(e: Event) => setDescription((e.target as HTMLInputElement).value)}
          />
          <wf-select
            value={retentionDays()}
            on:change={(e: Event) => setRetentionDays((e.target as HTMLSelectElement).value)}
          >
            {RETENTION_OPTIONS.map(o => <option value={o.value}>{o.label}</option>)}
          </wf-select>
          <div style="display: flex; justify-content: flex-end; gap: var(--wf-space-sm);">
            <wf-button variant="text" on:click={() => { setEditing(false); editDialogRef?.hide?.(); }}>Cancel</wf-button>
            <wf-button on:click={handleSave}>Save</wf-button>
          </div>
        </div>
      </wf-dialog>

      {/* Delete confirmation dialog */}
      <wf-dialog ref={deleteDialogRef} header="Delete Project" on:wf-close={() => setConfirmDelete(false)}>
        <div style="padding: var(--wf-space-sm);">
          <p>Delete <strong>{props.project.name}</strong>? This cannot be undone.</p>
          <div style="display: flex; justify-content: flex-end; gap: var(--wf-space-sm); margin-top: var(--wf-space-md);">
            <wf-button variant="text" on:click={() => { setConfirmDelete(false); deleteDialogRef?.hide?.(); }}>
              Cancel
            </wf-button>
            <wf-button on:click={handleDelete}>Delete</wf-button>
          </div>
        </div>
      </wf-dialog>
    </div>
  );
}
