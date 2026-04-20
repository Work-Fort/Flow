import { describe, it, expect } from 'vitest';
import { mount, unmount, manifest } from '../src/index';

describe('flow remote contract', () => {
  it('exports the ServiceModule manifest', () => {
    expect(manifest.name).toBe('flow');
    expect(manifest.label).toBe('Flow');
    expect(manifest.route).toBe('/flow');
  });
  it('mounts and unmounts without throwing', () => {
    const el = document.createElement('div');
    mount(el, { connected: true });
    expect(el.children.length).toBeGreaterThan(0);
    unmount(el);
    expect(el.children.length).toBe(0);
  });
});
