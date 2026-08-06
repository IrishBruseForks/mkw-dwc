#!/usr/bin/env node
// Keyboard automation API for two open MKWii Dolphin windows.
"use strict";

const { spawnSync } = require("node:child_process");

const REPO_ROOT = require("node:path").join(__dirname, "..");
const USER_ROOT =
	process.env.MKWII_DOLPHIN_DIR ||
	require("node:path").join(REPO_ROOT, "tmp/dolphin-test");

const DPAD_DIRS = new Set(["Left", "Right", "Up", "Down"]);
const TARGETS = new Set(["p1", "p2", "both"]);

let windowBackend = "x11";
let activeSession = null;

function needSession() {
	if (!activeSession) die("call createSession() first");
	return activeSession;
}

function die(msg) {
	const err = new Error(msg);
	err.fatal = true;
	throw err;
}

function log(...args) {
	console.log(...args);
}

function sleep(ms) {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

function hasCmd(cmd) {
	return (
		spawnSync("sh", ["-c", `command -v -- ${JSON.stringify(cmd)}`], {
			stdio: "ignore",
		}).status === 0
	);
}

function need(cmd) {
	if (!hasCmd(cmd)) die(`missing command: ${cmd}`);
}

function capture(cmd, args) {
	return spawnSync(cmd, args, {
		encoding: "utf8",
		stdio: ["ignore", "pipe", "pipe"],
	});
}

function isWayland() {
	return process.env.XDG_SESSION_TYPE === "wayland";
}

function isCinnamon() {
	const desk = `${process.env.XDG_CURRENT_DESKTOP || ""}${process.env.DESKTOP_SESSION || ""}`;
	if (/cinnamon/i.test(desk)) return true;
	return capture("pgrep", ["-x", "cinnamon"]).status === 0;
}

function initBackends() {
	need("xdotool");
	if (isWayland()) {
		if (!isCinnamon()) {
			die(
				"Wayland automation needs Cinnamon (org.Cinnamon Eval). Use an X11 session.",
			);
		}
		need("gdbus");
		need("python3");
		windowBackend = "cinnamon";
	} else {
		windowBackend = "x11";
	}
}

function cinnamonEval(script) {
	const py = `
import ast
import json
import subprocess
import sys

script = sys.argv[1]
r = subprocess.run(
    [
        "gdbus", "call", "--session", "--dest", "org.Cinnamon",
        "--object-path", "/org/Cinnamon", "--method", "org.Cinnamon.Eval",
        script,
    ],
    capture_output=True, text=True, check=True,
)
ok, val = ast.literal_eval(
    r.stdout.strip().replace("true", "True").replace("false", "False")
)
if not ok:
    sys.exit(1)
try:
    parsed = json.loads(val)
    if isinstance(parsed, str):
        try:
            parsed = json.loads(parsed)
        except json.JSONDecodeError:
            pass
    print(json.dumps(parsed))
except (json.JSONDecodeError, TypeError):
    print(json.dumps(val))
`;
	const r = spawnSync("python3", ["-c", py, script], { encoding: "utf8" });
	if (r.status !== 0) die(`cinnamon eval failed: ${r.stderr || r.stdout}`);
	return JSON.parse(r.stdout.trim());
}

function mkwWindowsCinnamon() {
	return cinnamonEval(`
let wins = global.display.get_tab_list(0, null)
	.filter(w => w.get_title().includes("Mario Kart Wii"))
	.map(w => {
		let r = w.get_frame_rect();
		return {id: w.get_id(), title: w.get_title(), x: r.x, y: r.y, w: r.width, h: r.height};
	})
	.sort((a, b) => a.x - b.x)
	.slice(0, 2);
JSON.stringify(wins);
`);
}

function mkwWids() {
	const r = capture("xdotool", ["search", "--name", "Mario Kart Wii"]);
	if (r.status !== 0) return [];
	const ids = r.stdout.trim().split("\n").filter(Boolean);
	const out = [];
	for (const wid of ids) {
		const name = capture("xdotool", ["getwindowname", wid]).stdout.trim();
		if (name.includes("Mario Kart Wii")) out.push(wid);
	}
	return out;
}

function mkwWidsLtr() {
	const wids = [...new Set(mkwWids())];
	if (wids.length < 2) return [];
	const ranked = wids.map((wid) => {
		const g = capture("xdotool", ["getwindowgeometry", wid]).stdout;
		const m = g.match(/Position: (\d+),/);
		const x = m ? Number(m[1]) : 0;
		return { x, wid };
	});
	ranked.sort((a, b) => a.x - b.x);
	return ranked.slice(0, 2).map((r) => r.wid);
}

function mkwPairIds() {
	if (windowBackend === "cinnamon") {
		return mkwWindowsCinnamon().map((w) => String(w.id));
	}
	return mkwWidsLtr();
}

function sendKey(x11Id, key) {
	capture("xdotool", ["key", "--window", x11Id, "--clearmodifiers", key]);
}

function readEnv() {
	return {
		bootWait: Number(process.env.MKWII_BOOT_WAIT || 0),
		skipA: Number(process.env.MKWII_SKIP_AS || 18),
		aInterval: Number(process.env.MKWII_A_INTERVAL || 0.7),
		aKey: process.env.MKWII_A_KEY || "x",
		dpadDownCount: Number(process.env.MKWII_DPAD_DOWN_COUNT || 2),
		dpadToWfc: process.env.MKWII_DPAD_TO_WFC || "Left",
		dpadToWfcCount: Number(process.env.MKWII_DPAD_TO_WFC_COUNT || 2),
	};
}

/**
 * @typedef {"p1"|"p2"|"both"} WindowTarget
 */

async function waitForWindows(timeoutSec = 90) {
	initBackends();
	const deadline = Date.now() + timeoutSec * 1000;
	while (Date.now() < deadline) {
		const cinnamonIds = mkwPairIds();
		const x11Ids = mkwWidsLtr();
		if (cinnamonIds.length >= 2 && x11Ids.length >= 2) {
			return { cinnamonIds, x11Ids };
		}
		await sleep(1500);
	}
	die(`timed out after ${timeoutSec}s waiting for 2 Mario Kart Wii windows`);
}

function resolveTargets(session, target) {
	if (!TARGETS.has(target)) die(`bad target ${target} (want p1|p2|both)`);
	if (target === "p1") return [session.p1.x11Id];
	if (target === "p2") return [session.p2.x11Id];
	return [session.p1.x11Id, session.p2.x11Id];
}

async function createSession(opts = {}) {
	const windowWait = Number(
		opts.windowWait ?? process.env.MKWII_WINDOW_WAIT ?? 0,
	);
	if (windowWait > 0) {
		log(`waiting up to ${windowWait}s for two Mario Kart Wii windows...`);
		await waitForWindows(windowWait);
	}
	initBackends();
	const cinnamonIds = mkwPairIds();
	if (cinnamonIds.length < 2) {
		die(`need 2 open Mario Kart Wii windows (found ${cinnamonIds.length})`);
	}
	const x11Ids = mkwWidsLtr();
	if (x11Ids.length < 2) {
		die(`need 2 X11 Mario Kart Wii windows (found ${x11Ids.length})`);
	}

	log(`backends: window=${windowBackend} input=xdotool-keyboard (no focus switch)`);
	log(
		`using open windows: P1=${cinnamonIds[0]} (x11=${x11Ids[0]}) P2=${cinnamonIds[1]} (x11=${x11Ids[1]})`,
	);

	const aKey = opts.aKey || process.env.MKWII_A_KEY || "x";

	const session = {
		p1: { cinnamonId: cinnamonIds[0], x11Id: x11Ids[0] },
		p2: { cinnamonId: cinnamonIds[1], x11Id: x11Ids[1] },
		aKey,
	};

	activeSession = session;
	return session;
}

async function key(target, keyName) {
	const session = needSession();
	const xids = resolveTargets(session, target);
	const labels = target === "both" ? ["p1", "p2"] : [target];
	for (let i = 0; i < xids.length; i++) {
		log(`key ${labels[i]} x11=${xids[i]} ${keyName}`);
		sendKey(xids[i], keyName);
	}
}

async function pressA(target, times = 1, intervalMs = 0) {
	const session = needSession();
	for (let i = 0; i < times; i++) {
		await key(target, session.aKey);
		await sleep(80);
		if (intervalMs > 0 && i + 1 < times) await sleep(intervalMs);
	}
}

async function dpad(target, dir, times = 1) {
	if (!DPAD_DIRS.has(dir)) {
		die(`bad dpad direction ${dir} (want Left|Right|Up|Down)`);
	}
	for (let i = 0; i < times; i++) {
		await key(target, dir);
		await sleep(120);
	}
}

const flows = {
	async nintendoWfc(opts) {
		const session = needSession();
		const o = { ...readEnv(), ...opts };
		if (!DPAD_DIRS.has(o.dpadToWfc)) {
			die(
				`MKWII_DPAD_TO_WFC must be Left, Right, Up, or Down (got: ${o.dpadToWfc})`,
			);
		}

		if (o.bootWait > 0) {
			log(`waiting ${o.bootWait}s before automation (MKWII_BOOT_WAIT)...`);
			await sleep(o.bootWait * 1000);
		}

		log(
			`menu navigate both: ${o.skipA} skip-A (${session.aKey}) @ ${o.aInterval}s`,
		);
		await pressA("both", o.skipA, o.aInterval * 1000);

		log(`license A (${session.aKey})`);
		await pressA("both");
		await sleep(2500);

		log(
			`D-pad Down x${o.dpadDownCount}, then ${o.dpadToWfc} x${o.dpadToWfcCount} -> Nintendo WFC`,
		);
		await dpad("both", "Down", o.dpadDownCount);
		await dpad("both", o.dpadToWfc, o.dpadToWfcCount);
		await sleep(300);
		await pressA("both");
		await sleep(800);
		log(
			`Nintendo WFC open attempted on both windows (P1=${session.p1.cinnamonId} P2=${session.p2.cinnamonId})`,
		);
	},
};

function usage() {
	console.log("Usage: node scripts/automate.js");
	console.log("  Drive Nintendo WFC menus on two open Mario Kart Wii windows.");
	console.log("  Env:");
	console.log("    MKWII_DOLPHIN_DIR       user dirs root (default: tmp/dolphin-test)");
	console.log("    MKWII_WINDOW_WAIT       seconds to wait for windows (default: 0)");
	console.log("    MKWII_BOOT_WAIT         seconds before automation (default: 0)");
	console.log("    MKWII_SKIP_AS           A key presses (default: 18)");
	console.log("    MKWII_A_INTERVAL        seconds between skip-A (default: 0.7)");
	console.log("    MKWII_A_KEY             Wiimote A key (default: x)");
	console.log("    MKWII_DPAD_DOWN_COUNT   D-pad Down before WFC row (default: 2)");
	console.log("    MKWII_DPAD_TO_WFC       horizontal D-pad (default: Left)");
	console.log("    MKWII_DPAD_TO_WFC_COUNT horizontal taps (default: 2)");
	process.exit(0);
}

async function main() {
	const arg = process.argv[2];
	if (arg === "-h" || arg === "--help") usage();
	if (arg) die(`unknown argument: ${arg} (try: node scripts/automate.js --help)`);

	await createSession();
	await flows.nintendoWfc(readEnv());
}

module.exports = {
	createSession,
	key,
	pressA,
	dpad,
	sleep,
	flows,
	readEnv,
	waitForWindows,
	DPAD_DIRS,
	get p1() {
		return activeSession?.p1;
	},
	get p2() {
		return activeSession?.p2;
	},
	get aKey() {
		return activeSession?.aKey;
	},
};

if (require.main === module) {
	main().catch((err) => {
		console.error(`error: ${err.message || String(err)}`);
		process.exit(1);
	});
}
