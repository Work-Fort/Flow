// SPDX-License-Identifier: GPL-2.0-only
import { render } from 'solid-js/web';
import { describe, it, expect, vi } from 'vitest';
import { ProjectList } from '../../src/components/project-list';

describe('ProjectList', () => {
  it('renders rows for each project', async () => {
    vi.mock('../../src/client', () => ({
      flowAPI: {
        get: () => Promise.resolve([
          { id: 'prj_1', name: 'Flow', channel_name: '#flow' },
          { id: 'prj_2', name: 'Hive', channel_name: '#hive' },
        ]),
      },
    }));
    const el = document.createElement('div');
    render(() => <ProjectList onSelect={() => {}} />, el);
    await new Promise(r => setTimeout(r, 50));
    expect(el.textContent).toContain('Flow');
    expect(el.textContent).toContain('Hive');
  });
});
