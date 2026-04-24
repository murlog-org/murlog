// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { sanitize } from "./sanitize";

describe("sanitize", () => {
	it("allows safe inline tags", () => {
		const html = "<p>hello <em>world</em> <strong>!</strong></p>";
		expect(sanitize(html)).toBe(html);
	});

	it("allows links with href, rel, class", () => {
		const html =
			'<a href="https://example.com" rel="nofollow" class="link">link</a>';
		expect(sanitize(html)).toBe(html);
	});

	it("allows lists", () => {
		const html = "<ul><li>one</li><li>two</li></ul>";
		expect(sanitize(html)).toBe(html);
	});

	it("allows blockquote, pre, code", () => {
		const html = "<blockquote><pre><code>x</code></pre></blockquote>";
		expect(sanitize(html)).toBe(html);
	});

	it("strips script tags", () => {
		expect(sanitize('<script>alert("xss")</script>')).not.toContain("<script>");
	});

	it("strips img tags", () => {
		expect(sanitize('<img src="x.png">')).not.toContain("<img");
	});

	it("strips iframe tags", () => {
		expect(sanitize('<iframe src="https://evil.com"></iframe>')).not.toContain(
			"<iframe",
		);
	});

	it("strips style tags", () => {
		expect(sanitize("<style>body{display:none}</style>")).not.toContain(
			"<style>",
		);
	});

	it("strips event handler attributes", () => {
		const result = sanitize('<p onclick="alert(1)">text</p>');
		expect(result).not.toContain("onclick");
		expect(result).toContain("<p>text</p>");
	});

	it("strips style attribute", () => {
		const result = sanitize('<p style="color:red">text</p>');
		expect(result).not.toContain("style");
	});

	it("strips target attribute from links", () => {
		const result = sanitize(
			'<a href="https://example.com" target="_blank">link</a>',
		);
		expect(result).not.toContain("target");
	});

	it("strips disallowed attributes but keeps allowed ones", () => {
		const result = sanitize(
			'<a href="https://x.com" id="bad" class="ok">link</a>',
		);
		expect(result).toContain('href="https://x.com"');
		expect(result).toContain('class="ok"');
		expect(result).not.toContain("id=");
	});

	it("handles empty string", () => {
		expect(sanitize("")).toBe("");
	});

	it("handles plain text without tags", () => {
		expect(sanitize("hello world")).toBe("hello world");
	});

	it("strips nested dangerous content", () => {
		const result = sanitize("<div><script>evil()</script><p>safe</p></div>");
		expect(result).not.toContain("<script>");
		expect(result).toContain("safe");
	});
});
