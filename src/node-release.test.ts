import { describe, expect, test } from "bun:test";
import { parseNodeReleaseVersion } from "./node-release";

describe("node release", () => {
  test("accepts semantic release versions", () => {
    expect(parseNodeReleaseVersion({ version: "0.12.0" })).toBe("0.12.0");
  });

  test("rejects invalid release metadata", () => {
    expect(parseNodeReleaseVersion({ version: "latest" })).toBe("");
    expect(parseNodeReleaseVersion(null)).toBe("");
  });
});
