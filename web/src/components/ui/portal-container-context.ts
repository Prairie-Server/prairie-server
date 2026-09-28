import * as React from "react";

/**
 * Provides a DOM element that Popovers should use as their portal container
 * when rendered inside a Sheet or Dialog.
 *
 * Radix Dialog locks scroll on everything outside its own content subtree via
 * react-remove-scroll. Portaling into an element that lives inside the Dialog
 * content keeps scroll events in the allowed zone.
 *
 * When null (no provider), Popovers fall back to document.body as normal.
 */
export const PortalContainerContext = React.createContext<HTMLElement | null>(null);
