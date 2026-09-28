/** Safe display conversion for unknown values (avoids `[object Object]`). */
export function stringifyUnknown(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
    return String(value);
  }
  if (typeof value === "symbol")
    return value.description ? `Symbol(${value.description})` : "Symbol()";
  if (value instanceof Error) return value.message || value.name;
  try {
    return JSON.stringify(value) ?? "";
  } catch {
    return Object.prototype.toString.call(value);
  }
}
