// SPDX-License-Identifier: GPL-2.0-only
import { createEffect, createSignal, For } from 'solid-js';
import type { Project, CreateProjectBody } from '../stores/projects';
import type { Vocabulary } from '../stores/vocabularies';

interface Props {
  open: boolean;
  vocabularies: Vocabulary[];
  onSubmit: (body: CreateProjectBody) => Promise<void>;
  onClose: () => void;
}

const RETENTION_OPTIONS = [
  { label: 'Permanent', value: '' },
  { label: '30 days', value: '30' },
  { label: '90 days', value: '90' },
  { label: '180 days', value: '180' },
  { label: '365 days', value: '365' },
];

export function ProjectCreateDialog(props: Props) {
  let dialogRef!: HTMLElement & { show(): void; hide(): void };
  const [name, setName] = createSignal('');
  const [description, setDescription] = createSignal('');
  const [channelName, setChannelName] = createSignal('');
  const [vocabularyId, setVocabularyId] = createSignal('');
  const [retentionDays, setRetentionDays] = createSignal('');
  const [channelAlreadyExists, setChannelAlreadyExists] = createSignal(false);
  const [submitting, setSubmitting] = createSignal(false);

  createEffect(() => {
    if (props.open) dialogRef?.show?.();
    else dialogRef?.hide?.();
  });

  async function handleSubmit() {
    if (!name().trim()) return;
    setSubmitting(true);
    try {
      const body: CreateProjectBody = {
        name: name().trim(),
        description: description().trim() || undefined,
        channel_name: channelName().trim() || undefined,
        vocabulary_id: vocabularyId() || undefined,
        retention_days: retentionDays() ? parseInt(retentionDays()) : null,
        channel_already_exists: channelAlreadyExists() || undefined,
      };
      await props.onSubmit(body);
      setName('');
      setDescription('');
      setChannelName('');
      setVocabularyId('');
      setRetentionDays('');
      setChannelAlreadyExists(false);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <wf-dialog ref={dialogRef} header="Create Project" on:wf-close={props.onClose}>
      <div style="display: flex; flex-direction: column; gap: var(--wf-space-md); padding: var(--wf-space-sm);">
        <wf-input
          placeholder="Project name *"
          value={name()}
          on:input={(e: Event) => setName((e.target as HTMLInputElement).value)}
        />
        <wf-input
          placeholder="Description"
          value={description()}
          on:input={(e: Event) => setDescription((e.target as HTMLInputElement).value)}
        />
        <wf-input
          placeholder="Sharkfin channel (e.g. #flow)"
          value={channelName()}
          on:input={(e: Event) => setChannelName((e.target as HTMLInputElement).value)}
        />
        <wf-select
          value={vocabularyId()}
          on:change={(e: Event) => setVocabularyId((e.target as HTMLSelectElement).value)}
        >
          <option value="">-- Vocabulary --</option>
          <For each={props.vocabularies}>
            {(v) => <option value={v.id}>{v.name}</option>}
          </For>
        </wf-select>
        <wf-select
          value={retentionDays()}
          on:change={(e: Event) => setRetentionDays((e.target as HTMLSelectElement).value)}
        >
          <For each={RETENTION_OPTIONS}>
            {(o) => <option value={o.value}>{o.label}</option>}
          </For>
        </wf-select>
        <details>
          <summary style="font-size: var(--wf-text-sm); color: var(--wf-color-text-muted); cursor: pointer;">
            Advanced
          </summary>
          <label style="display: flex; align-items: center; gap: var(--wf-space-sm); font-size: var(--wf-text-sm); margin-top: var(--wf-space-sm);">
            <input
              type="checkbox"
              checked={channelAlreadyExists()}
              on:change={(e: Event) => setChannelAlreadyExists((e.target as HTMLInputElement).checked)}
            />
            Channel already exists in Sharkfin
          </label>
        </details>
        <div style="display: flex; justify-content: flex-end; gap: var(--wf-space-sm);">
          <wf-button variant="text" on:click={props.onClose}>Cancel</wf-button>
          <wf-button on:click={handleSubmit} disabled={submitting() || !name().trim()}>
            {submitting() ? 'Creating…' : 'Create'}
          </wf-button>
        </div>
      </div>
    </wf-dialog>
  );
}
