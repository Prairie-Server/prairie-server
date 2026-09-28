import type * as React from "react";

type VidstackElementProps = React.DetailedHTMLProps<
  React.HTMLAttributes<HTMLElement>,
  HTMLElement
> & {
  "aria-label"?: string;
  "aria-hidden"?: boolean | "true" | "false";
  placement?: string;
  offset?: number | string;
  orientation?: string;
  type?: string;
  event?: string;
  action?: string;
  value?: string | number;
  min?: number | string;
  max?: number | string;
  step?: number | string;
  "key-step"?: number | string;
  noClamp?: boolean;
  "no-clamp"?: boolean;
  "off-label"?: string;
  disabled?: boolean;
  autoPlay?: boolean;
  viewType?: string;
  streamType?: string;
  crossOrigin?: string;
  playsInline?: boolean;
  duration?: number;
  src?: string;
  kind?: string;
  label?: string;
  srcLang?: string;
  default?: boolean;
  children?: React.ReactNode;
};

type VidstackIntrinsicElements = {
  "media-player": VidstackElementProps;
  "media-provider": VidstackElementProps;
  "media-controls": VidstackElementProps;
  "media-controls-group": VidstackElementProps;
  "media-play-button": VidstackElementProps;
  "media-mute-button": VidstackElementProps;
  "media-caption-button": VidstackElementProps;
  "media-pip-button": VidstackElementProps;
  "media-fullscreen-button": VidstackElementProps;
  "media-time-slider": VidstackElementProps;
  "media-slider-chapters": VidstackElementProps;
  "media-slider-preview": VidstackElementProps;
  "media-slider-thumbnail": VidstackElementProps;
  "media-slider-value": VidstackElementProps;
  "media-volume-slider": VidstackElementProps;
  "media-time": VidstackElementProps;
  "media-menu": VidstackElementProps;
  "media-menu-button": VidstackElementProps;
  "media-menu-item": VidstackElementProps;
  "media-menu-items": VidstackElementProps;
  "media-menu-portal": VidstackElementProps;
  "media-radio": VidstackElementProps;
  "media-captions-radio-group": VidstackElementProps;
  "media-speed-slider": VidstackElementProps;
  "media-quality-slider": VidstackElementProps;
  "media-tooltip": VidstackElementProps;
  "media-tooltip-trigger": VidstackElementProps;
  "media-tooltip-content": VidstackElementProps;
  "media-captions": VidstackElementProps;
  "media-spinner": VidstackElementProps;
  "media-gesture": VidstackElementProps;
  track: React.DetailedHTMLProps<React.TrackHTMLAttributes<HTMLTrackElement>, HTMLTrackElement>;
  template: React.DetailedHTMLProps<React.HTMLAttributes<HTMLTemplateElement>, HTMLTemplateElement>;
  source: React.DetailedHTMLProps<React.SourceHTMLAttributes<HTMLSourceElement>, HTMLSourceElement>;
};

declare global {
  namespace JSX {
    interface IntrinsicElements extends VidstackIntrinsicElements {}
  }
}

declare module "react" {
  namespace JSX {
    interface IntrinsicElements extends VidstackIntrinsicElements {}
  }
}

declare module "react/jsx-runtime" {
  namespace JSX {
    interface IntrinsicElements extends VidstackIntrinsicElements {}
  }
}

export {};
