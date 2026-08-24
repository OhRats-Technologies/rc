export class HttpError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export function requireValue<T>(value: T | null | undefined, status: number, message: string): T {
  if (value === null || value === undefined) throw new HttpError(status, message);
  return value;
}
