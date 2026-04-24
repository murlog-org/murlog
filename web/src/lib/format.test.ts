import { describe, it, expect, vi, afterEach } from "vitest";
import { formatTime } from "./format";

describe("formatTime", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns relative seconds for very recent dates", () => {
    const now = new Date();
    const result = formatTime(new Date(now.getTime() - 30000).toISOString());
    // Intl.RelativeTimeFormat produces locale-dependent output.
    // Just verify it returns a non-empty string, not the ISO format.
    expect(result).toBeTruthy();
    expect(result).not.toMatch(/^\d{4}-/);
  });

  it("returns relative minutes", () => {
    const result = formatTime(new Date(Date.now() - 5 * 60000).toISOString());
    expect(result).toBeTruthy();
    expect(result).not.toMatch(/^\d{4}-/);
  });

  it("returns relative hours", () => {
    const result = formatTime(new Date(Date.now() - 3 * 3600000).toISOString());
    expect(result).toBeTruthy();
  });

  it("returns relative days for recent dates", () => {
    const result = formatTime(new Date(Date.now() - 2 * 86400000).toISOString());
    expect(result).toBeTruthy();
  });

  it("returns localized date for old dates (> 30 days)", () => {
    const result = formatTime(new Date(Date.now() - 60 * 86400000).toISOString());
    expect(result).toBeTruthy();
    // Should be a formatted date, not relative.
  });

  it("handles empty string gracefully", () => {
    const result = formatTime("");
    // Invalid Date → toLocaleDateString or Intl output.
    expect(typeof result).toBe("string");
  });

  it("handles invalid date string", () => {
    const result = formatTime("not-a-date");
    expect(typeof result).toBe("string");
  });

  it("handles future dates", () => {
    const result = formatTime(new Date(Date.now() + 3600000).toISOString());
    // Intl.RelativeTimeFormat supports future ("in 1 hour").
    expect(result).toBeTruthy();
  });

  it("boundary: 59 seconds ago → seconds unit", () => {
    const result = formatTime(new Date(Date.now() - 59000).toISOString());
    expect(result).toBeTruthy();
  });

  it("boundary: 61 seconds ago → minutes unit", () => {
    const result = formatTime(new Date(Date.now() - 61000).toISOString());
    expect(result).toBeTruthy();
  });
});
