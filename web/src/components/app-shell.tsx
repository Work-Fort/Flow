// SPDX-License-Identifier: GPL-2.0-only
import { createSignal, Show } from 'solid-js';

type Screen = 'projects' | 'work-items' | 'agents' | 'audit';

export function AppShell(props: { connected: boolean }) {
  const [screen, setScreen] = createSignal<Screen>('projects');
  return (
    <div>
      <Show when={!props.connected}>
        <wf-banner variant="warning" headline="Flow service unreachable" />
      </Show>
      <nav>
        <wf-button onClick={() => setScreen('projects')}>Projects</wf-button>
        <wf-button onClick={() => setScreen('work-items')}>Work Items</wf-button>
        <wf-button onClick={() => setScreen('agents')}>Agents</wf-button>
        <wf-button onClick={() => setScreen('audit')}>Audit</wf-button>
      </nav>
      <main>
        <p>Screen: {screen()}</p>
      </main>
    </div>
  );
}
