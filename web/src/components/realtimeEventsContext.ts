import { createContext, useContext, useEffect } from "react";
import type { AdminJob, EventChannel } from "@/api/types";

export type RealtimeConnectionState = "connecting" | "live" | "disconnected";

export interface EventChannelHandlers {
  onSnapshot?: (message: unknown) => void;
  onEvent?: (message: unknown) => void;
}

export interface RealtimeEventsContextValue {
  connectionState: RealtimeConnectionState;
  awaitAdminJob: (jobId: string) => Promise<AdminJob>;
  subscribeChannel: (channel: EventChannel, handlers?: EventChannelHandlers) => () => void;
}

export const RealtimeEventsContext = createContext<RealtimeEventsContextValue>({
  connectionState: "disconnected",
  awaitAdminJob: async () => {
    throw new Error("Realtime events provider is not mounted");
  },
  subscribeChannel: () => () => {},
});

export function useRealtimeEvents() {
  return useContext(RealtimeEventsContext);
}

export function useEventChannel(channel: EventChannel, handlers?: EventChannelHandlers) {
  const { subscribeChannel } = useRealtimeEvents();

  useEffect(() => subscribeChannel(channel, handlers), [channel, handlers, subscribeChannel]);
}
