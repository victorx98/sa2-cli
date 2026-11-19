import { spawn } from "node:child_process";
import { join } from "node:path";

// Only the fixed script goes in argv. The session payload travels over pipes.
export async function protectWindows(
  data: Buffer,
  operation: "Protect" | "Unprotect",
): Promise<Buffer> {
  const script = `
    $ErrorActionPreference = 'Stop'
    Add-Type -AssemblyName System.Security
    $data = [Convert]::FromBase64String([Console]::In.ReadToEnd())
    $result = [Security.Cryptography.ProtectedData]::${operation}(
      $data, $null, [Security.Cryptography.DataProtectionScope]::CurrentUser)
    [Console]::Out.Write([Convert]::ToBase64String($result))
  `;
  const powershell = join(
    process.env.SystemRoot ?? "C:\\Windows",
    "System32",
    "WindowsPowerShell",
    "v1.0",
    "powershell.exe",
  );
  return new Promise((resolve, reject) => {
    const child = spawn(
      powershell,
      [
        "-NoLogo",
        "-NoProfile",
        "-NonInteractive",
        "-EncodedCommand",
        Buffer.from(script, "utf16le").toString("base64"),
      ],
      { windowsHide: true, timeout: 10_000, stdio: ["pipe", "pipe", "ignore"] },
    );
    let output = "";
    child.stdout.setEncoding("ascii");
    child.stdout.on("data", (chunk: string) => {
      output += chunk;
    });
    child.on("error", reject);
    child.stdin.on("error", reject);
    child.on("close", (code) => {
      if (code === 0 && /^[A-Za-z0-9+/]+={0,2}$/.test(output)) {
        resolve(Buffer.from(output, "base64"));
      } else {
        reject(new Error("Windows credential protection failed."));
      }
    });
    child.stdin.end(data.toString("base64"));
  });
}
