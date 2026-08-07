#!/usr/bin/env node
// Launch two Dolphin clients, then WFC menu automation (mkw-dwc must already run).
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createSession,
  pressA,
  dpad,
  sleep,
  step,
  run,
  readEnv,
  DPAD_DIRS,
  getP1,
  getP2,
  getAKey
} from "./automate.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(__dirname, "..");
const LAUNCH_JS = join(__dirname, "launch.js");

function die(msg) {
  console.error(`error: ${msg}`);
  process.exit(1);
}

function log(...args) {
  console.log(...args);
}

function nintendoWfc(opts = {}) {
  const o = { ...readEnv(), ...opts };
  if (o.dpadToWfcCount > 0 && !DPAD_DIRS.has(o.dpadToWfc)) {
    die(
      `MKWII_DPAD_TO_WFC must be Left, Right, Up, or Down (got: ${o.dpadToWfc})`
    );
  }

  const phaseMs = o.phaseWait * 1000;

  step(() =>
    log(`waiting ${o.bootWait}s for logos/title (MKWII_BOOT_WAIT)...`)
  );
  sleep(o.bootWait * 1000);

  if (o.skipA > 0) {
    step(() =>
      log(`skip logos/title A x${o.skipA} (${getAKey()}) @ ${o.aInterval}s`)
    );
    pressA("both", o.skipA, o.aInterval * 1000);
    sleep(phaseMs);
  }

  step(() =>
    log(`phase: title Press A (${getAKey()}), then wait ${o.phaseWait}s`)
  );
  pressA("both", 1);
  sleep(phaseMs);

  step(() =>
    log(`phase: license select A (${getAKey()}), then wait ${o.phaseWait}s`)
  );
  pressA("both", 1);
  sleep(phaseMs);

  step(() =>
    log(
      `phase: main menu -> Nintendo WFC (Down x${o.dpadDownCount}` +
        (o.dpadToWfcCount > 0 ? `, ${o.dpadToWfc} x${o.dpadToWfcCount}` : "") +
        ")"
    )
  );
  dpad("both", "Down", o.dpadDownCount);
  sleep(800);
  if (o.dpadToWfcCount > 0) {
    dpad("both", o.dpadToWfc, o.dpadToWfcCount);
    sleep(500);
  }
  pressA("both", 1);
  sleep(phaseMs);

  step(() => {
    const p1 = getP1();
    const p2 = getP2();
    log(
      `Nintendo WFC open attempted (P1=${p1.cinnamonId} P2=${p2.cinnamonId})`
    );
  });
}

function usage() {
  console.log("Usage: node scripts/test.js [path/to/Mario Kart Wii.iso]");
  console.log("  ISO may also come from MKWII_ISO.");
  console.log("  Env:");
  console.log(
    "    MKWII_TILE=1          tile both windows on the primary monitor"
  );
  console.log(
    "    MKWII_BOOT_WAIT       seconds before first input (default: 45)"
  );
  console.log(
    "    MKWII_PHASE_WAIT      seconds between menu phases (default: 5)"
  );
  console.log(
    "    MKWII_WINDOW_WAIT     seconds to wait for windows (default: 120)"
  );
  process.exit(1);
}

function waitExit(child) {
  return new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => resolve({ code, signal }));
  });
}

function sleepMs(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function main() {
  const arg = process.argv[2];
  if (arg === "-h" || arg === "--help") usage();

  const iso = arg || process.env.MKWII_ISO || "";
  if (!iso) die("pass an ISO path or set MKWII_ISO");
  if (!existsSync(iso)) die(`ISO not found: ${iso}`);

  if (!process.env.MKWII_WINDOW_WAIT) {
    process.env.MKWII_WINDOW_WAIT = "120";
  }

  log("launching Dolphin clients (background)...");
  const launch = spawn(process.execPath, [LAUNCH_JS, iso], {
    stdio: "inherit",
    cwd: REPO_ROOT,
    env: process.env
  });

  let shuttingDown = false;
  const shutdown = async (exitCode = 0) => {
    if (shuttingDown) return;
    shuttingDown = true;
    if (launch.exitCode === null && !launch.killed) {
      launch.kill("SIGTERM");
      try {
        await Promise.race([
          waitExit(launch),
          sleepMs(2000).then(() => {
            if (launch.exitCode === null && !launch.killed) {
              launch.kill("SIGKILL");
            }
          })
        ]);
      } catch {
        /* ignore */
      }
    }
    process.exit(exitCode);
  };

  process.on("SIGINT", () => {
    void shutdown(0);
  });
  process.on("SIGTERM", () => {
    void shutdown(0);
  });

  try {
    log(
      "waiting for two Mario Kart Wii windows, then running menu automation..."
    );
    createSession();
    await run();
  } catch (err) {
    console.error(`error: ${err.message || String(err)}`);
    await shutdown(1);
  }

  log("automation done; Ctrl+C stops Dolphin clients");
  const { code, signal } = await waitExit(launch);
  if (signal) process.exit(0);
  process.exit(code ?? 0);
}

main().catch((err) => die(err.message || String(err)));
