import { createContext, useContext } from "react";
import type { PlayerPictureInPictureChange } from "@/player";
import type {
  WatchPlaybackStartInput,
  WatchRouteRequest,
} from "@/pages/watchRouteHelpers";
import type {
  WatchPlaybackHostState,
  WatchPlaybackSnapshot,
  WatchPlaybackTransportControls,
} from "./watchPlaybackReducer";

export interface WatchPlaybackControllerValue {
  state: WatchPlaybackHostState;
  hasDetachedPlayback: boolean;
  isBackgroundBarVisible: boolean;
  startPlayback: (input: WatchPlaybackStartInput | WatchRouteRequest) => void;
  minimizePlayback: () => void;
  exitPlayback: (options?: { destinationHref?: string }) => void;
  stopPlayback: () => void;
  enterPostRoll: (requestKey: string) => void;
  returnToWatch: () => void;
  syncRouteRequest: (request: WatchRouteRequest) => void;
  handleRouteExit: (requestKey: string) => void;
  setPictureInPictureActive: (
    requestKey: string,
    change: PlayerPictureInPictureChange,
  ) => void;
  clearPendingReturnNavigation: (requestKey: string) => void;
  updatePlaybackSnapshot: (
    requestKey: string,
    snapshot: WatchPlaybackSnapshot,
  ) => void;
  setTransportControls: (
    requestKey: string,
    controls: WatchPlaybackTransportControls | null,
  ) => void;
}

export const WatchPlaybackControllerContext =
  createContext<WatchPlaybackControllerValue | null>(null);

export function useWatchPlaybackController() {
  const context = useContext(WatchPlaybackControllerContext);
  if (!context) {
    throw new Error(
      "Watch playback controller is unavailable outside WatchPlaybackProvider",
    );
  }
  return context;
}
