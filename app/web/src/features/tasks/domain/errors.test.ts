import { describe, expect, it } from "vitest";
import { InvalidTransitionError } from "./errors";

describe("InvalidTransitionError", () => {
  it("is a typed Error carrying the attempted from/to transition", () => {
    const error = new InvalidTransitionError("done", "doing");

    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(InvalidTransitionError);
    expect(error.name).toBe("InvalidTransitionError");
    expect(error.from).toBe("done");
    expect(error.to).toBe("doing");
    expect(error.message).toBe('Cannot transition task from "done" to "doing"');
  });
});
