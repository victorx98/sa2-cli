#!/usr/bin/env node
import { Command, CommanderError } from "commander";
import promptPassword from "@inquirer/password";
import { apiOrigin, CliError, createAuth } from "@sa2-cli/core";

function output(result: unknown): void {
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

async function readPassword(fromStdin: boolean): Promise<string> {
  if (fromStdin) {
    if (process.stdin.isTTY) {
      throw new CliError(
        "INVALID_ARGUMENT",
        "Pipe the password to --password-stdin.",
        2,
      );
    }
    process.stdin.setEncoding("utf8");
    let value = "";
    for await (const chunk of process.stdin) {
      value += chunk;
      if (value.length > 64 * 1024) {
        throw new CliError(
          "INVALID_ARGUMENT",
          "Password input is too large.",
          2,
        );
      }
    }
    return value.replace(/\r?\n$/, "");
  }

  if (!process.stdin.isTTY || !process.stderr.isTTY) {
    throw new CliError(
      "INVALID_ARGUMENT",
      "Use --password-stdin when running without a terminal.",
      2,
    );
  }
  return promptPassword(
    { message: "Password" },
    { input: process.stdin, output: process.stderr },
  );
}

const program = new Command()
  .name("sa2")
  .description("SuperAcademy command-line interface")
  .version("0.1.0")
  .option("--api-url <origin>", "SA2 API origin (defaults to SA2_API_URL)")
  .configureOutput({ writeErr: () => undefined })
  .exitOverride();

function auth() {
  return createAuth(
    apiOrigin(
      program.opts<{ apiUrl?: string }>().apiUrl ?? process.env.SA2_API_URL,
    ),
  );
}

program
  .command("login")
  .description("Sign in using an email and password")
  .requiredOption("--email <email>")
  .option("--password-stdin", "Read the password from standard input")
  .action(async (options: { email: string; passwordStdin?: boolean }) => {
    const email = options.email.trim();
    if (!/^[^\s@]+@[^\s@]+$/.test(email)) {
      throw new CliError(
        "INVALID_ARGUMENT",
        "Provide a valid email address.",
        2,
      );
    }
    const session = auth();
    const password = await readPassword(options.passwordStdin ?? false);
    if (!password)
      throw new CliError("INVALID_ARGUMENT", "A password is required.", 2);
    output({ data: await session.login(email, password) });
  });

program
  .command("whoami")
  .description("Show the authenticated user")
  .action(async () => output({ data: await auth().whoami() }));

program
  .command("logout")
  .description("Clear the local session for this API origin")
  .action(async () => output({ data: await auth().logout() }));

try {
  await program.parseAsync();
} catch (error) {
  if (!(error instanceof CommanderError && error.exitCode === 0)) {
    let result = new CliError(
      "INTERNAL_ERROR",
      "The command could not be completed.",
    );
    if (error instanceof CliError) result = error;
    if (error instanceof CommanderError) {
      result = new CliError(
        "INVALID_ARGUMENT",
        "Check the command and options with --help.",
        2,
      );
    }
    if (error instanceof Error && error.name === "ExitPromptError") {
      result = new CliError("CANCELLED", "Login cancelled.", 130);
    }
    process.exitCode = result.exitCode;
    output({ error: { code: result.code, message: result.message } });
  }
}
