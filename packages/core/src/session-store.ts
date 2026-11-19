import { createHash, randomUUID } from "node:crypto";
import { lstat, mkdir, open, rename, rm } from "node:fs/promises";
import { homedir } from "node:os";
import { isAbsolute, join } from "node:path";
import lockfile from "proper-lockfile";
import { isSession, type Session } from "./api/client.js";
import { protectWindows } from "./dpapi.js";
import { CliError } from "./errors.js";

const windows = process.platform === "win32";

function storageError() {
  return new CliError(
    "SESSION_STORAGE_ERROR",
    "Could not access the protected SA2 session store.",
  );
}

function stateDirectory(): string {
  if (windows) {
    const directory = process.env.LOCALAPPDATA;
    if (!directory || !isAbsolute(directory)) throw storageError();
    return join(directory, "sa2-cli");
  }
  if (process.platform === "darwin")
    return join(homedir(), "Library", "Application Support", "sa2-cli");
  const state = process.env.XDG_STATE_HOME;
  return join(
    state && isAbsolute(state) ? state : join(homedir(), ".local", "state"),
    "sa2-cli",
  );
}

function isMissing(error: unknown): boolean {
  return (error as NodeJS.ErrnoException).code === "ENOENT";
}

export class SessionStore {
  private readonly directory = stateDirectory();
  private readonly path: string;

  constructor(private readonly origin: string) {
    const key = createHash("sha256").update(origin).digest("hex");
    this.path = join(this.directory, `${key}.session`);
  }

  async locked<T>(operation: () => Promise<T>): Promise<T> {
    let release: () => Promise<void>;
    let compromised = false;
    try {
      await mkdir(this.directory, { recursive: true, mode: 0o700 });
      const info = await lstat(this.directory);
      if (
        !info.isDirectory() ||
        (!windows &&
          ((info.mode & 0o077) !== 0 || info.uid !== process.getuid?.()))
      ) {
        throw storageError();
      }
      release = await lockfile.lock(this.path, {
        realpath: false,
        stale: 30_000,
        update: 5_000,
        retries: { retries: 60, factor: 1, minTimeout: 250 },
        onCompromised: () => {
          compromised = true;
        },
      });
    } catch {
      throw storageError();
    }
    try {
      const result = await operation();
      if (compromised) throw storageError();
      return result;
    } finally {
      await release().catch(() => {
        throw storageError();
      });
    }
  }

  async read(): Promise<Session | null> {
    try {
      const info = await lstat(this.path);
      if (
        !info.isFile() ||
        (!windows &&
          ((info.mode & 0o077) !== 0 || info.uid !== process.getuid?.()))
      ) {
        throw storageError();
      }
      const file = await open(this.path, "r");
      const bytes = await file.readFile().finally(() => file.close());
      const contents = windows
        ? await protectWindows(bytes, "Unprotect")
        : bytes;
      const value: unknown = JSON.parse(contents.toString("utf8"));
      if (
        !value ||
        typeof value !== "object" ||
        !("origin" in value) ||
        value.origin !== this.origin ||
        !("session" in value) ||
        !isSession(value.session)
      ) {
        throw storageError();
      }
      return value.session;
    } catch (error) {
      if (isMissing(error)) return null;
      throw storageError();
    }
  }

  async write(session: Session): Promise<void> {
    const temp = `${this.path}.${randomUUID()}.tmp`;
    try {
      const contents = Buffer.from(
        JSON.stringify({ origin: this.origin, session }),
      );
      const data = windows
        ? await protectWindows(contents, "Protect")
        : contents;
      const file = await open(temp, "wx", 0o600);
      try {
        await file.writeFile(data);
        await file.sync();
      } finally {
        await file.close();
      }
      await rename(temp, this.path);
    } catch {
      throw storageError();
    } finally {
      await rm(temp, { force: true }).catch(() => undefined);
    }
  }

  async clear(): Promise<void> {
    try {
      await rm(this.path, { force: true });
    } catch {
      throw storageError();
    }
  }
}
