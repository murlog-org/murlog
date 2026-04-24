import { describe, it, expect } from "vitest";
import { extractHost } from "./queue";

// Minimal Job type for testing.
type TestJob = { id: string; type: string; payload: string; status: string; attempts: number; last_error: string; next_run_at: string; created_at: string };

function makeJob(payload: string): TestJob {
  return { id: "1", type: "deliver_note", payload, status: "pending", attempts: 0, last_error: "", next_run_at: "", created_at: "" };
}

describe("extractHost", () => {
  it("extracts host from actor_uri", () => {
    expect(extractHost(makeJob('{"actor_uri":"https://mastodon.social/users/alice"}'))).toBe("mastodon.social");
  });

  it("extracts host from target_actor_uri", () => {
    expect(extractHost(makeJob('{"target_actor_uri":"https://gts.example.com/users/bob"}'))).toBe("gts.example.com");
  });

  it("extracts host from target_uri", () => {
    expect(extractHost(makeJob('{"target_uri":"https://misskey.io/users/xyz"}'))).toBe("misskey.io");
  });

  it("prefers actor_uri over target_actor_uri", () => {
    expect(extractHost(makeJob('{"actor_uri":"https://a.com/u/x","target_actor_uri":"https://b.com/u/y"}'))).toBe("a.com");
  });

  it("returns empty for empty payload", () => {
    expect(extractHost(makeJob(""))).toBe("");
  });

  it("returns empty for invalid JSON", () => {
    expect(extractHost(makeJob("not json"))).toBe("");
    expect(extractHost(makeJob("{ incomplete"))).toBe("");
  });

  it("returns empty for JSON without URI fields", () => {
    expect(extractHost(makeJob('{"name":"job"}'))).toBe("");
  });

  it("returns empty for invalid URI in field", () => {
    expect(extractHost(makeJob('{"actor_uri":"not-a-url"}'))).toBe("");
  });

  it("handles URI with port", () => {
    expect(extractHost(makeJob('{"actor_uri":"https://localhost:3000/users/a"}'))).toBe("localhost");
  });

  it("handles URI with subdomain", () => {
    expect(extractHost(makeJob('{"actor_uri":"https://ap.sub.example.co.uk/users/a"}'))).toBe("ap.sub.example.co.uk");
  });
});
