import { describe, expect, it } from "vitest";
import { acctFromURI } from "../components/post-card";

describe("acctFromURI", () => {
	it("converts standard Mastodon URI", () => {
		expect(acctFromURI("https://mastodon.social/users/alice")).toBe(
			"@alice@mastodon.social",
		);
	});

	it("converts GoToSocial URI", () => {
		expect(acctFromURI("https://gts.example.com/users/bob")).toBe(
			"@bob@gts.example.com",
		);
	});

	it("converts URI with port", () => {
		expect(acctFromURI("https://localhost:3000/users/alice")).toBe(
			"@alice@localhost:3000",
		);
	});

	it("handles URI with nested path", () => {
		// Misskey-like: /users/internal-id — takes last segment.
		expect(acctFromURI("https://misskey.io/users/akptweh1lz3s02mg")).toBe(
			"@akptweh1lz3s02mg@misskey.io",
		);
	});

	it("handles trailing slash", () => {
		// pop() on trailing slash gives empty string.
		const result = acctFromURI("https://example.com/users/alice/");
		expect(result).toBe("@@example.com");
	});

	it("handles URI with no path segments", () => {
		const result = acctFromURI("https://example.com");
		expect(result).toBeTruthy();
	});

	it("returns input for invalid URL", () => {
		expect(acctFromURI("not-a-url")).toBe("not-a-url");
	});

	it("returns input for empty string", () => {
		expect(acctFromURI("")).toBe("");
	});

	it("handles Lemmy-style AP URI", () => {
		expect(acctFromURI("https://lemmy.world/u/username")).toBe(
			"@username@lemmy.world",
		);
	});
});
