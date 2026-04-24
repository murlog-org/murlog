import { describe, expect, it } from "vitest";
import { autoLinkText } from "./theme";

describe("autoLinkText", () => {
	it("wraps https URL in <a> tag", () => {
		const result = autoLinkText("https://example.com");
		expect(result).toContain('<a href="https://example.com"');
		expect(result).toContain('target="_blank"');
		expect(result).toContain('rel="nofollow noopener"');
	});

	it("wraps http URL in <a> tag", () => {
		const result = autoLinkText("http://example.com");
		expect(result).toContain('<a href="http://example.com"');
	});

	it("wraps URL with path and query", () => {
		const result = autoLinkText("https://example.com/path?q=1&b=2");
		expect(result).toContain("<a");
		expect(result).toContain("example.com/path");
	});

	it("does NOT wrap plain text", () => {
		const result = autoLinkText("just plain text");
		expect(result).not.toContain("<a");
		expect(result).toBe("just plain text");
	});

	it("does NOT wrap text without protocol", () => {
		expect(autoLinkText("example.com")).not.toContain("<a");
		expect(autoLinkText("www.example.com")).not.toContain("<a");
	});

	it("does NOT wrap URL with leading space", () => {
		expect(autoLinkText(" https://example.com")).not.toContain("<a");
	});

	it("does NOT wrap URL with trailing space", () => {
		expect(autoLinkText("https://example.com ")).not.toContain("<a");
	});

	it("escapes HTML in URL", () => {
		const result = autoLinkText("https://example.com/<script>alert()</script>");
		expect(result).not.toContain("<script>");
		expect(result).toContain("&lt;script&gt;");
	});

	it("escapes HTML in plain text", () => {
		const result = autoLinkText('<script>alert("xss")</script>');
		expect(result).not.toContain("<script>");
		expect(result).toContain("&lt;script&gt;");
	});

	it("handles empty string", () => {
		expect(autoLinkText("")).toBe("");
	});

	it("handles URL with unicode domain", () => {
		const result = autoLinkText("https://例え.jp/path");
		expect(result).toContain("<a");
	});
});
