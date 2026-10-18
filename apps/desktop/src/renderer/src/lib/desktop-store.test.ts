import { describe, expect, it } from "vitest";
import { DEFAULT_STATUS } from "@daybee/shared";
import { statusTone } from "./status-tone";

describe("statusTone", () => {
  it("maps recording status to tone", () => {
    expect(
      statusTone({
        ...DEFAULT_STATUS,
        elapsed: "00:00:01",
        recording: true,
        statusMessage: ""
      })
    ).toBe("recording");
  });
});
