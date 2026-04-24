import { describe, expect, it } from "vitest";
import {
	ERR_CONFLICT,
	ERR_NOT_FOUND,
	ERR_UNAUTHORIZED,
	isUnauthorized,
} from "./api";

describe("isUnauthorized", () => {
	it("returns true for ERR_UNAUTHORIZED code", () => {
		expect(
			isUnauthorized({ code: ERR_UNAUTHORIZED, message: "auth required" }),
		).toBe(true);
	});

	it("returns false for other error codes", () => {
		expect(isUnauthorized({ code: ERR_NOT_FOUND, message: "not found" })).toBe(
			false,
		);
		expect(isUnauthorized({ code: ERR_CONFLICT, message: "conflict" })).toBe(
			false,
		);
		expect(isUnauthorized({ code: -32603, message: "internal" })).toBe(false);
	});

	it("returns false for undefined", () => {
		expect(isUnauthorized(undefined)).toBe(false);
	});

	it("returns false for error without code match", () => {
		expect(isUnauthorized({ code: 0, message: "" })).toBe(false);
	});
});

describe("error code constants", () => {
	it("has expected values matching server-side", () => {
		expect(ERR_UNAUTHORIZED).toBe(-32000);
		expect(ERR_NOT_FOUND).toBe(-32001);
		expect(ERR_CONFLICT).toBe(-32002);
	});
});
