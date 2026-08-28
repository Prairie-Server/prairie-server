import { describe, expect, it } from "vitest";

import {
  parseBucketPolicies,
  serializeBucketPolicies,
  type LogRetentionBucketPolicy,
} from "./logRetentionPolicy";

describe("logRetentionPolicy", () => {
  it("parses valid bucket policies and normalizes levels", () => {
    const policies = parseBucketPolicies(
      JSON.stringify([
        {
          component: "metadata",
          level: "INFO",
          retention_days: "1",
          max_rows: "100000",
          max_size_mb: "128",
        },
      ]),
    );

    expect(policies).toEqual<LogRetentionBucketPolicy[]>([
      {
        component: "metadata",
        level: "info",
        retention_days: 1,
        max_rows: 100000,
        max_size_mb: 128,
      },
    ]);
  });

  it("drops invalid policies when parsing", () => {
    const policies = parseBucketPolicies(
      JSON.stringify([
        {
          component: "",
          level: "info",
          retention_days: 1,
          max_rows: 10,
          max_size_mb: 10,
        },
        {
          component: "scanner",
          level: "verbose",
          retention_days: 1,
          max_rows: 10,
          max_size_mb: 10,
        },
      ]),
    );

    expect(policies).toEqual([]);
  });

  it("preserves zero for disabled limits and normalizes invalid numeric fields to 0", () => {
    const policies = parseBucketPolicies(
      JSON.stringify([
        {
          component: "metadata",
          level: "info",
          retention_days: 0,
          max_rows: -5,
          max_size_mb: "",
        },
      ]),
    );

    expect(policies).toEqual<LogRetentionBucketPolicy[]>([
      {
        component: "metadata",
        level: "info",
        retention_days: 0,
        max_rows: 0,
        max_size_mb: 0,
      },
    ]);
  });

  it("serializes disabled limits as 0", () => {
    const raw = serializeBucketPolicies([
      {
        component: "metadata",
        level: "info",
        retention_days: 0,
        max_rows: 0,
        max_size_mb: 0,
      },
    ]);

    expect(JSON.parse(raw)).toEqual([
      {
        component: "metadata",
        level: "info",
        retention_days: 0,
        max_rows: 0,
        max_size_mb: 0,
      },
    ]);
  });

  it("serializes only valid policies", () => {
    const raw = serializeBucketPolicies([
      {
        component: " metadata ",
        level: "INFO",
        retention_days: 1,
        max_rows: 100000,
        max_size_mb: 128,
      },
      {
        component: "",
        level: "warn",
        retention_days: 7,
        max_rows: 250000,
        max_size_mb: 256,
      },
    ]);

    expect(JSON.parse(raw)).toEqual([
      {
        component: "metadata",
        level: "info",
        retention_days: 1,
        max_rows: 100000,
        max_size_mb: 128,
      },
    ]);
  });
});
