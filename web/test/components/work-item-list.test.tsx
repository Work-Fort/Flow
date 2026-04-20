// SPDX-License-Identifier: GPL-2.0-only
import { render } from 'solid-js/web';
import { describe, it, expect, vi } from 'vitest';
import { WorkItemList } from '../../src/components/work-item-list';

describe('WorkItemList', () => {
  it('renders rows for each work item', async () => {
    vi.mock('../../src/client', () => ({
      flowAPI: {
        get: () => Promise.resolve([
          { id: 'wi_1', title: 'Fix bug', status: 'in_progress', instance_id: 'inst_1', updated_at: '2026-01-01T00:00:00Z', created_at: '2026-01-01T00:00:00Z' },
          { id: 'wi_2', title: 'Write tests', status: 'done', instance_id: 'inst_2', updated_at: '2026-01-01T00:00:00Z', created_at: '2026-01-01T00:00:00Z' },
        ]),
      },
    }));
    const el = document.createElement('div');
    render(() => <WorkItemList projectId="prj_1" onSelect={() => {}} />, el);
    await new Promise(r => setTimeout(r, 50));
    expect(el.textContent).toContain('Fix bug');
    expect(el.textContent).toContain('Write tests');
  });
});
