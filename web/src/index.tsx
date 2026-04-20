// SPDX-License-Identifier: GPL-2.0-only
import { render } from 'solid-js/web';
import { AppShell } from './components/app-shell';

const roots = new WeakMap<HTMLElement, () => void>();

export function mount(el: HTMLElement, props: { connected: boolean }) {
  const dispose = render(() => <AppShell connected={props.connected} />, el);
  roots.set(el, dispose);
}

export function unmount(el: HTMLElement) {
  const dispose = roots.get(el);
  if (dispose) {
    dispose();
    roots.delete(el);
  }
}

export const manifest = {
  name: 'flow' as const,
  label: 'Flow' as const,
  route: '/flow' as const,
  display: 'nav' as const,
};
