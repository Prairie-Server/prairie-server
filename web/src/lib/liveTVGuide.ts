export function channelDisplayNumber(channel: {
  number: string;
  number_override?: string | null;
}): string {
  const override = channel.number_override?.trim();
  return override || channel.number;
}

export function channelLabel(channel: {
  number: string;
  number_override?: string | null;
  callsign: string;
  name: string;
}): string {
  const number = channelDisplayNumber(channel);
  const name = channel.callsign || channel.name || "Channel";
  return `${number} · ${name}`;
}

export type NowNext = {
  now: { id: string; title: string; start: string; stop: string } | null;
  next: { id: string; title: string; start: string; stop: string } | null;
};

/** Pick the current and following programme for a channel from a flat guide list. */
export function pickNowNext(
  programs: Array<{ id: string; channel_id: string; title: string; start: string; stop: string }>,
  channelId: string,
  now: Date = new Date(),
): NowNext {
  const sorted = programs
    .filter((p) => p.channel_id === channelId)
    .slice()
    .sort((a, b) => Date.parse(a.start) - Date.parse(b.start));

  let current: (typeof sorted)[number] | null = null;
  let upcoming: (typeof sorted)[number] | null = null;
  for (const program of sorted) {
    const start = Date.parse(program.start);
    const stop = Date.parse(program.stop);
    if (Number.isNaN(start) || Number.isNaN(stop)) continue;
    if (start <= now.getTime() && stop > now.getTime()) {
      current = program;
      continue;
    }
    if (start > now.getTime()) {
      upcoming = program;
      break;
    }
  }
  if (!upcoming && current) {
    const idx = sorted.findIndex((p) => p.id === current.id);
    upcoming = idx >= 0 ? (sorted[idx + 1] ?? null) : null;
  }
  return {
    now: current
      ? { id: current.id, title: current.title, start: current.start, stop: current.stop }
      : null,
    next: upcoming
      ? { id: upcoming.id, title: upcoming.title, start: upcoming.start, stop: upcoming.stop }
      : null,
  };
}

export function formatGuideTime(iso: string, locale?: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" });
}

export type GuideWindow = {
  startMs: number;
  endMs: number;
  /** Pixels per millisecond for absolute program layout. */
  pxPerMs: number;
};

export const GUIDE_HOUR_WIDTH_PX = 240;

/** Build a guide layout window snapped to half-hour boundaries. */
export function buildGuideWindow(
  now: Date = new Date(),
  pastHours = 0.5,
  futureHours = 6,
): GuideWindow {
  const halfHour = 30 * 60 * 1000;
  const rawStart = now.getTime() - pastHours * 60 * 60 * 1000;
  const startMs = Math.floor(rawStart / halfHour) * halfHour;
  const endMs = startMs + (pastHours + futureHours) * 60 * 60 * 1000;
  return {
    startMs,
    endMs,
    pxPerMs: GUIDE_HOUR_WIDTH_PX / (60 * 60 * 1000),
  };
}

export function guideTimeTicks(window: GuideWindow, stepMinutes = 30): number[] {
  const step = stepMinutes * 60 * 1000;
  const ticks: number[] = [];
  for (let t = window.startMs; t < window.endMs; t += step) {
    ticks.push(t);
  }
  return ticks;
}

export type GuideProgramLayout = {
  id: string;
  channelId: string;
  title: string;
  subtitle?: string;
  start: string;
  stop: string;
  leftPx: number;
  widthPx: number;
  isNow: boolean;
};

/** Position programmes absolutely within a guide window for one channel. */
export function layoutProgramsForChannel(
  programs: Array<{
    id: string;
    channel_id: string;
    title: string;
    subtitle?: string;
    start: string;
    stop: string;
  }>,
  channelId: string,
  window: GuideWindow,
  now: Date = new Date(),
): GuideProgramLayout[] {
  const nowMs = now.getTime();
  const laid: GuideProgramLayout[] = [];
  for (const program of programs) {
    if (program.channel_id !== channelId) continue;
    const start = Date.parse(program.start);
    const stop = Date.parse(program.stop);
    if (
      Number.isNaN(start) ||
      Number.isNaN(stop) ||
      stop <= window.startMs ||
      start >= window.endMs
    ) {
      continue;
    }
    const clampedStart = Math.max(start, window.startMs);
    const clampedStop = Math.min(stop, window.endMs);
    laid.push({
      id: program.id,
      channelId,
      title: program.title,
      subtitle: program.subtitle,
      start: program.start,
      stop: program.stop,
      leftPx: (clampedStart - window.startMs) * window.pxPerMs,
      widthPx: Math.max(48, (clampedStop - clampedStart) * window.pxPerMs),
      isNow: start <= nowMs && stop > nowMs,
    });
  }
  laid.sort((a, b) => a.leftPx - b.leftPx);
  return laid;
}

export function progressFraction(
  startIso: string,
  stopIso: string,
  now: Date = new Date(),
): number {
  const start = Date.parse(startIso);
  const stop = Date.parse(stopIso);
  if (Number.isNaN(start) || Number.isNaN(stop) || stop <= start) return 0;
  return Math.min(1, Math.max(0, (now.getTime() - start) / (stop - start)));
}
