// SPDX-License-Identifier: GPL-2.0-only
import { createSignal } from 'solid-js';
import type { Project, PatchProjectBody } from '../stores/projects';
import { flowAPI } from '../client';

interface Props {
  project: Project;
  onUpdated: (p: Project) => void;
}

const RETENTION_OPTIONS = [
  { label: 'Permanent', value: '' },
  { label: '30 days', value: '30' },
  { label: '90 days', value: '90' },
  { label: '180 days', value: '180' },
  { label: '365 days', value: '365' },
];

export function RetentionControls(props: Props) {
  const [saving, setSaving] = createSignal(false);

  async function handleChange(e: Event) {
    const val = (e.target as HTMLSelectElement).value;
    setSaving(true);
    try {
      const body: PatchProjectBody = val
        ? { retention_days: parseInt(val) }
        : { clear_retention_days: true };
      const updated = await flowAPI.patch<Project>(`/v1/projects/${props.project.id}`, body);
      props.onUpdated(updated);
    } finally {
      setSaving(false);
    }
  }

  const currentValue = () =>
    props.project.retention_days != null ? String(props.project.retention_days) : '';

  return (
    <div>
      <wf-banner
        variant="info"
        headline="Retention is recorded but not yet enforced"
      >
        Events are stored permanently. A future release will add scheduled purging.
      </wf-banner>
      <div style="margin-top: var(--wf-space-sm); display: flex; align-items: center; gap: var(--wf-space-sm);">
        <label style="font-size: var(--wf-text-sm); color: var(--wf-color-text-secondary);">
          Audit retention:
        </label>
        <wf-select
          value={currentValue()}
          disabled={saving()}
          on:change={handleChange}
        >
          {RETENTION_OPTIONS.map(o => <option value={o.value}>{o.label}</option>)}
        </wf-select>
      </div>
    </div>
  );
}
