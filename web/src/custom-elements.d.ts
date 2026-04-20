// Ambient declarations for @workfort/ui custom elements used as JSX tags.
// Solid-js JSX uses JSX.IntrinsicElements from solid-js/jsx-runtime.
import type { JSX } from 'solid-js';

type WfBannerProps = JSX.HTMLAttributes<HTMLElement> & {
  variant?: 'info' | 'warning' | 'error';
  headline?: string;
};

type WfButtonProps = JSX.HTMLAttributes<HTMLElement> & {
  variant?: string;
  title?: string;
  slot?: string;
};

declare module 'solid-js' {
  namespace JSX {
    interface IntrinsicElements {
      'wf-banner': WfBannerProps;
      'wf-button': WfButtonProps;
    }
  }
}
