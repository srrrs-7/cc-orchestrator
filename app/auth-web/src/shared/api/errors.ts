export type ApiErrorOptions = {
  readonly status: number;
  readonly cause?: unknown;
};

/** Normalized error shape for anything that goes wrong talking to the API. */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, options: ApiErrorOptions) {
    super(message, options.cause === undefined ? undefined : { cause: options.cause });
    this.name = "ApiError";
    this.status = options.status;
  }
}
