// Ambient declarations for @workfort/ui custom elements used as JSX tags.
// Solid-js JSX uses JSX.IntrinsicElements from solid-js/jsx-runtime.
import type { JSX } from 'solid-js';

type WfBaseProps = JSX.HTMLAttributes<HTMLElement>;

type WfBannerProps = WfBaseProps & {
  variant?: 'info' | 'warning' | 'error';
  headline?: string;
};

type WfButtonProps = WfBaseProps & {
  variant?: string;
  title?: string;
  slot?: string;
  disabled?: boolean;
};

type WfInputProps = WfBaseProps & {
  placeholder?: string;
  value?: string;
  type?: string;
};

type WfSelectProps = WfBaseProps & {
  value?: string;
};

type WfListProps = WfBaseProps;

type WfListItemProps = WfBaseProps & {
  active?: boolean;
};

type WfDialogProps = WfBaseProps & {
  header?: string;
  open?: boolean;
};

type WfSkeletonProps = WfBaseProps;

type WfBadgeProps = WfBaseProps & {
  count?: number;
  size?: string;
  'data-wf'?: string;
};

declare module 'solid-js' {
  namespace JSX {
    interface IntrinsicElements {
      'wf-banner': WfBannerProps;
      'wf-button': WfButtonProps;
      'wf-input': WfInputProps;
      'wf-select': WfSelectProps;
      'wf-list': WfListProps;
      'wf-list-item': WfListItemProps;
      'wf-dialog': WfDialogProps;
      'wf-skeleton': WfSkeletonProps;
      'wf-badge': WfBadgeProps;
    }
  }
}
