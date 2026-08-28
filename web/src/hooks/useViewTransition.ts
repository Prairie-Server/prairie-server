import { useNavigate } from "react-router";
import { useCallback } from "react";
import type { NavigateOptions, To } from "react-router";

/**
 * A wrapper around React Router's `useNavigate` that uses the
 * View Transitions API for smooth page transitions.
 *
 * Falls back to regular navigation in browsers that don't support
 * the View Transitions API.
 */
export function useViewTransitionNavigate() {
  const navigate = useNavigate();

  return useCallback(
    (to: To, options?: NavigateOptions) => {
      void navigate(to, { ...options, viewTransition: true });
    },
    [navigate],
  );
}
