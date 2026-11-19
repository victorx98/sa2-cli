export class CliError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly exitCode = 1,
  ) {
    super(message);
  }
}

export function loginRequired(): CliError {
  return new CliError(
    "LOGIN_REQUIRED",
    "Run sa2 login to start a new session.",
    3,
  );
}
