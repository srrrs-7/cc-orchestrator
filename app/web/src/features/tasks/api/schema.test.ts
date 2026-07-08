import { describe, expect, it } from "vitest";
import type { Task } from "../domain/task";
import type { TaskDto } from "./schema";
import { createTaskRequestSchema, taskListSchema, taskSchema, toDomain, toDto } from "./schema";

const validDto: TaskDto = {
  id: "1",
  title: "Set up project scaffolding",
  status: "done",
  priority: "high",
  created_at: "2026-07-01T09:00:00.000Z",
  updated_at: "2026-07-01T09:30:00.000Z",
};

describe("taskSchema", () => {
  it("parses a valid DTO (normal)", () => {
    expect(taskSchema.parse(validDto)).toEqual(validDto);
  });

  it("parses a list of valid DTOs (normal)", () => {
    expect(taskListSchema.parse([validDto])).toEqual([validDto]);
  });

  it("fails when a required field is missing (abnormal)", () => {
    const { title, ...withoutTitle } = validDto;
    void title;
    const result = taskSchema.safeParse(withoutTitle);
    expect(result.success).toBe(false);
  });

  it("fails for an unrecognized status enum value (abnormal)", () => {
    const result = taskSchema.safeParse({ ...validDto, status: "archived" });
    expect(result.success).toBe(false);
  });

  it("fails for an unrecognized priority enum value (abnormal)", () => {
    const result = taskSchema.safeParse({ ...validDto, priority: "urgent" });
    expect(result.success).toBe(false);
  });
});

describe("toDomain / toDto round trip", () => {
  it("maps a DTO (snake_case) to a domain Task (camelCase)", () => {
    const task = toDomain(validDto);

    const expected: Task = {
      id: validDto.id,
      title: validDto.title,
      status: validDto.status,
      priority: validDto.priority,
      createdAt: validDto.created_at,
      updatedAt: validDto.updated_at,
    };
    expect(task).toEqual(expected);
  });

  it("maps a domain Task back to its DTO form", () => {
    const task = toDomain(validDto);
    expect(toDto(task)).toEqual(validDto);
  });

  it("round-trips DTO -> domain -> DTO without loss", () => {
    expect(toDto(toDomain(validDto))).toEqual(validDto);
  });
});

describe("createTaskRequestSchema", () => {
  it("parses a valid create-task request (normal)", () => {
    const result = createTaskRequestSchema.parse({ title: "Write docs", priority: "low" });
    expect(result).toEqual({ title: "Write docs", priority: "low" });
  });

  it("trims surrounding whitespace from the title", () => {
    const result = createTaskRequestSchema.parse({ title: "  Write docs  ", priority: "low" });
    expect(result.title).toBe("Write docs");
  });

  it("fails when the title is empty after trimming (abnormal)", () => {
    const result = createTaskRequestSchema.safeParse({ title: "   ", priority: "low" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.message).toBe("Title is required");
    }
  });

  it("accepts a title at exactly the 200 character limit (boundary)", () => {
    const title = "a".repeat(200);
    const result = createTaskRequestSchema.safeParse({ title, priority: "low" });
    expect(result.success).toBe(true);
  });

  it("fails when the title exceeds the 200 character limit (boundary)", () => {
    const title = "a".repeat(201);
    const result = createTaskRequestSchema.safeParse({ title, priority: "low" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.message).toBe("Title is too long");
    }
  });

  it("fails for an unrecognized priority enum value (abnormal)", () => {
    const result = createTaskRequestSchema.safeParse({ title: "Write docs", priority: "urgent" });
    expect(result.success).toBe(false);
  });
});
