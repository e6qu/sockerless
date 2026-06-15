import { describe, it, expect } from "vitest";
import { shortSHA, bytes } from "../format.js";

describe("format", () => {
  it("shortSHA truncates to 8 chars", () => {
    expect(shortSHA("40d51b0999fc518941b0558a6e2842a74240a10c")).toBe("40d51b09");
    expect(shortSHA("")).toBe("—");
  });
  it("bytes renders human sizes", () => {
    expect(bytes(0)).toBe("0 B");
    expect(bytes(512)).toBe("512 B");
    expect(bytes(2048)).toBe("2.0 KB");
    expect(bytes(5 * 1024 * 1024)).toBe("5.0 MB");
  });
});
