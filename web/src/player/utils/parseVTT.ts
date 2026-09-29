export interface ParsedCue {
  start: number;
  end: number;
  text: string;
}

/**
 * Parse a timestamp string (HH:MM:SS.mmm or MM:SS.mmm) into seconds.
 */
function parseTimestamp(ts: string): number {
  const parts = ts.trim().split(":");
  if (parts.length === 3) {
    return parseInt(parts[0]!, 10) * 3600 + parseInt(parts[1]!, 10) * 60 + parseFloat(parts[2]!);
  }
  if (parts.length === 2) {
    return parseInt(parts[0]!, 10) * 60 + parseFloat(parts[1]!);
  }
  return 0;
}

const TIMESTAMP_RE = /^([\d:.]+)\s*-->\s*([\d:.]+)/;

/**
 * Parse WebVTT text into an array of cue objects.
 */
export function parseVTT(vttText: string): ParsedCue[] {
  const cues: ParsedCue[] = [];
  const lines = vttText.split(/\r?\n/);

  let i = 0;

  // Skip the WEBVTT header and any metadata before the first cue.
  while (i < lines.length && !TIMESTAMP_RE.test(lines[i]!)) {
    i++;
  }

  while (i < lines.length) {
    const match = lines[i]!.match(TIMESTAMP_RE);
    if (!match) {
      i++;
      continue;
    }

    const start = parseTimestamp(match[1]!);
    const end = parseTimestamp(match[2]!);
    i++;

    // Collect text lines until a blank line or end of input.
    const textLines: string[] = [];
    while (i < lines.length && lines[i]!.trim() !== "") {
      // Skip numeric cue identifiers that precede timestamp lines.
      if (TIMESTAMP_RE.test(lines[i]!)) break;
      textLines.push(lines[i]!);
      i++;
    }

    const text = textLines.join("\n").trim();
    if (text) {
      cues.push({ start, end, text });
    }
  }

  return cues;
}
