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
