#!/usr/bin/env node
// Launch two Flatpak Dolphin MKWii instances, seed configs, tile windows.
"use strict";

const { spawn, spawnSync } = require("node:child_process");
const {
	copyFileSync,
	existsSync,
	mkdirSync,
	readFileSync,
	realpathSync,
	writeFileSync,
} = require("node:fs");
const { dirname, join } = require("node:path");

const REPO_ROOT = join(__dirname, "..");
const FLATPAK_APP = "org.DolphinEmu.dolphin-emu";
const USER_ROOT =
	process.env.MKWII_DOLPHIN_DIR || join(REPO_ROOT, "tmp/dolphin-test");
const GAME_IDS = ["RMCE01", "RMCJ01", "RMCP01"];
const NOSSL_NAME = "$NoSSL";
const NOSSL_LINES = [
	"C0000000 0000000E",
	"3C004E80 60000020",
	"900F0000 3D808000",
	"618C3000 3C00017F",
	"6000CFFC 7C0903A6",
	"3D607474 616B7073",
	"800C0000 7C005800",
	"40A20034 394C0003",
	"392C0002 7D455378",
	"38600000 8C050001",
	"2C000000 38630001",
	"4082FFF4 8C0A0001",
	"9C090001 3463FFFF",
	"4082FFF4 398C0001",
	"4200FFC0 4E800020",
];

const WIIMOTE_TEMPLATE = `[Wiimote1]
Device = XInput2/0/Virtual core pointer
Buttons/A = X | \`Click 1\`
Buttons/B = Z | \`Click 3\`
Buttons/1 = \`1\`
Buttons/2 = \`2\`
Buttons/- = Q
Buttons/+ = E
Buttons/Home = Return
D-Pad/Up = Up
D-Pad/Down = Down
D-Pad/Left = Left
D-Pad/Right = Right
IR/Up = I | \`Cursor Y-\`
IR/Down = K | \`Cursor Y+\`
IR/Left = J | \`Cursor X-\`
IR/Right = L | \`Cursor X+\`
Extension = None
[Wiimote2]
Device = XInput2/0/Virtual core pointer
[Wiimote3]
Device = XInput2/0/Virtual core pointer
[Wiimote4]
Device = XInput2/0/Virtual core pointer
[BalanceBoard]
Device = XInput2/0/Virtual core pointer
`;

const GCPAD_KEYBOARD_TEMPLATE = `[GCPad1]
Device = XInput2/0/Virtual core pointer
Buttons/A = X
Buttons/B = Z
Buttons/X = C
Buttons/Y = V
Buttons/Z = Shift_L
Buttons/Start = Return
Main Stick/Up = W
Main Stick/Down = S
Main Stick/Left = A
Main Stick/Right = D
C-Stick/Up = I
C-Stick/Down = K
C-Stick/Left = J
C-Stick/Right = L
Triggers/L = Q
Triggers/R = E
Triggers/L-Analog = Q
Triggers/R-Analog = E
D-Pad/Up = Up
D-Pad/Down = Down
D-Pad/Left = Left
D-Pad/Right = Right
[GCPad2]
Device = XInput2/0/Virtual core pointer
[GCPad3]
Device = XInput2/0/Virtual core pointer
[GCPad4]
Device = XInput2/0/Virtual core pointer
`;

let windowBackend = "x11";

function die(msg) {
	console.error(`error: ${msg}`);
	process.exit(1);
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

function initTilingBackends() {
	need("xdotool");
	if (isWayland()) {
		if (!isCinnamon()) {
			die(
				"Wayland tiling needs Cinnamon (org.Cinnamon Eval). Use X11 or tile manually.",
			);
		}
		need("gdbus");
		need("python3");
		windowBackend = "cinnamon";
	} else {
		need("wmctrl");
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

function writeNossl(iniPath) {
	mkdirSync(dirname(iniPath), { recursive: true });
	const body = [
		"[Core]",
		"EnableCheats = True",
		"",
		"[Gecko]",
		NOSSL_NAME,
		...NOSSL_LINES,
		"[Gecko_Enabled]",
		NOSSL_NAME,
		"",
	].join("\n");
	writeFileSync(iniPath, body);
}

function controllerConfigSrc() {
	const src =
		process.env.MKWII_CONTROLLER_CONFIG ||
		join(process.env.HOME, `.var/app/${FLATPAK_APP}/config/dolphin-emu`);
	if (existsSync(src)) return src;
	if (existsSync(join(src, "Config"))) return src;
	return "";
}

function controllerIniPath(src, name) {
	if (!src) return "";
	if (existsSync(join(src, name))) return join(src, name);
	if (existsSync(join(src, "Config", name))) return join(src, "Config", name);
	return "";
}

function orSectionBinds(iniPath, section, binds, formatLine) {
	if (!existsSync(iniPath)) return;
	const lines = readFileSync(iniPath, "utf8").split("\n");
	const out = [];
	let inSection = false;
	const seen = new Set();
	for (const line of lines) {
		const s = line.trim();
		if (s.startsWith("[") && s.endsWith("]")) {
			if (inSection) {
				for (const [k, v] of Object.entries(binds)) {
					if (!seen.has(k)) out.push(formatLine(k, v, ""));
				}
				seen.clear();
			}
			inSection = s === section;
			out.push(line);
			continue;
		}
		if (!inSection || !line.includes("=")) {
			out.push(line);
			continue;
		}
		const eq = line.indexOf("=");
		const key = line.slice(0, eq).trim();
		const val = line.slice(eq + 1).trim();
		if (!binds[key]) {
			out.push(line);
			continue;
		}
		seen.add(key);
		const kb = binds[key];
		const parts = val.split("|").map((p) => p.trim());
		if (parts.includes(kb) || parts.some((p) => p.includes(kb))) {
			out.push(line);
		} else if (!val) {
			out.push(formatLine(key, kb, val));
		} else {
			out.push(`${key} = ${kb} | ${val}`);
		}
	}
	if (inSection) {
		for (const [k, v] of Object.entries(binds)) {
			if (!seen.has(k)) out.push(formatLine(k, v, ""));
		}
	}
	writeFileSync(iniPath, out.join("\n") + "\n");
}

function orWiimoteKeyboard(iniPath) {
	const binds = {
		"Buttons/A": "X",
		"Buttons/B": "Z",
		"IR/Up": "I",
		"IR/Down": "K",
		"IR/Left": "J",
		"IR/Right": "L",
	};
	orSectionBinds(iniPath, "[Wiimote1]", binds, (k, v, val) =>
		val ? `${k} = ${v} | ${val}` : `${k} = ${v}`,
	);
}

function orGcpadKeyboard(iniPath) {
	const kb = "XInput2/0/Virtual core pointer";
	const binds = {
		"Buttons/A": "X",
		"Buttons/B": "Z",
		"Buttons/X": "C",
		"Buttons/Y": "V",
		"Buttons/Z": "Shift_L",
		"Buttons/Start": "Return",
		"Main Stick/Up": "W",
		"Main Stick/Down": "S",
		"Main Stick/Left": "A",
		"Main Stick/Right": "D",
		"C-Stick/Up": "I",
		"C-Stick/Down": "K",
		"C-Stick/Left": "J",
		"C-Stick/Right": "L",
		"Triggers/L": "Q",
		"Triggers/R": "E",
		"Triggers/L-Analog": "Q",
		"Triggers/R-Analog": "E",
		"D-Pad/Up": "Up",
		"D-Pad/Down": "Down",
		"D-Pad/Left": "Left",
		"D-Pad/Right": "Right",
	};
	orSectionBinds(iniPath, "[GCPad1]", binds, (k, v, val) => {
		const extra = `(\`${kb}:${v}\`)`;
		if (!val) return `${k} = ${extra}`;
		if (val.includes(`\`${kb}:${v}\``)) return `${k} = ${val}`;
		return `${k} = ${val} | ${extra}`;
	});
}

function ensureDolphinIniFlags(iniPath) {
	let text = existsSync(iniPath) ? readFileSync(iniPath, "utf8") : "";
	if (/^SIDevice0\s*=/m.test(text)) {
		text = text.replace(/^SIDevice0\s*=.*/m, "SIDevice0 = 6");
	} else if (/^\[Core\]/m.test(text)) {
		text = text.replace(/^\[Core\]/m, "[Core]\nSIDevice0 = 6");
	} else {
		text += "\n[Core]\nSIDevice0 = 6\n";
	}
	if (!/^BackgroundInput\s*=/m.test(text)) {
		if (/^\[Input\]/m.test(text)) {
			text = text.replace(/^\[Input\]/m, "[Input]\nBackgroundInput = True");
		} else {
			text += "\n[Input]\nBackgroundInput = True\n";
		}
	} else {
		text = text.replace(/^BackgroundInput\s*=.*/m, "BackgroundInput = True");
	}
	writeFileSync(iniPath, text);
}

function seedControllers(dir) {
	const configDir = join(dir, "Config");
	mkdirSync(configDir, { recursive: true });
	const src = controllerConfigSrc();
	let path = controllerIniPath(src, "GCPadNew.ini");
	if (path) {
		copyFileSync(path, join(configDir, "GCPadNew.ini"));
		orGcpadKeyboard(join(configDir, "GCPadNew.ini"));
	} else {
		writeFileSync(join(configDir, "GCPadNew.ini"), GCPAD_KEYBOARD_TEMPLATE);
	}
	path = controllerIniPath(src, "WiimoteNew.ini");
	if (path) {
		copyFileSync(path, join(configDir, "WiimoteNew.ini"));
		orWiimoteKeyboard(join(configDir, "WiimoteNew.ini"));
	} else {
		writeFileSync(join(configDir, "WiimoteNew.ini"), WIIMOTE_TEMPLATE);
	}
	ensureDolphinIniFlags(join(configDir, "Dolphin.ini"));
}

function seedUser(dir, mac, volume, videoBackend) {
	mkdirSync(join(dir, "Config"), { recursive: true });
	mkdirSync(join(dir, "GameSettings"), { recursive: true });
	writeFileSync(
		join(dir, "Config", "Dolphin.ini"),
		[
			"[Analytics]",
			"Enabled = False",
			"PermissionAsked = True",
			"[Core]",
			"EnableCheats = True",
			`GFXBackend = ${videoBackend}`,
			"SIDevice0 = 6",
			"SIDevice1 = 6",
			"[Interface]",
			"ConfirmStop = False",
			"RenderToMain = False",
			"OnScreenDisplayMessages = False",
			"PauseOnFocusLost = False",
			"[Input]",
			"BackgroundInput = True",
			"[DSP]",
			`Volume = ${volume}`,
			"[General]",
			`WirelessMac = ${mac}`,
			"",
		].join("\n"),
	);
	seedControllers(dir);
	for (const id of GAME_IDS) {
		writeNossl(join(dir, "GameSettings", `${id}.ini`));
	}
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

async function waitTwoWindows(timeoutS = 60) {
	const deadline = Date.now() + timeoutS * 1000;
	while (Date.now() < deadline) {
		const wids = mkwPairIds();
		if (wids.length >= 2) return wids.slice(0, 2);
		await sleep(400);
	}
	die("expected 2 Mario Kart Wii windows");
}

function leftMonitorGeom() {
	if (windowBackend === "cinnamon") {
		const m = cinnamonEval(`
let m = Main.layoutManager.monitors.slice().sort((a, b) => a.x - b.x)[0];
JSON.stringify({x: m.x, y: m.y, w: m.width, h: m.height});
`);
		return { ox: m.x, oy: m.y, sw: m.w, sh: m.h };
	}
	const r = capture("xrandr", ["--current"]);
	const re = /(\d+)x(\d+)\+(\d+)\+(\d+)/g;
	let best = null;
	let m;
	while ((m = re.exec(r.stdout)) !== null) {
		const w = Number(m[1]);
		const h = Number(m[2]);
		const x = Number(m[3]);
		const y = Number(m[4]);
		if (!best || x < best.ox) best = { ox: x, oy: y, sw: w, sh: h };
	}
	if (best) return best;
	const g = capture("xdotool", ["getdisplaygeometry"]).stdout.trim().split(/\s+/);
	return { ox: 0, oy: 0, sw: Number(g[0]), sh: Number(g[1]) };
}

function windowGeom(wid) {
	if (windowBackend === "cinnamon") {
		const r = cinnamonEval(`
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) "missing";
else {
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
`);
		return { x: r.x, y: r.y, w: r.w, h: r.h };
	}
	const r = capture("wmctrl", ["-lG"]).stdout.split("\n");
	for (const line of r) {
		const parts = line.trim().split(/\s+/);
		if (parts[0] === wid) {
			return {
				x: Number(parts[2]),
				y: Number(parts[3]),
				w: Number(parts[4]),
				h: Number(parts[5]),
			};
		}
	}
	die(`window geom missing for ${wid}`);
}

function windowActivate(wid) {
	if (windowBackend === "cinnamon") {
		cinnamonEval(`
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) 'missing';
else {
	w.activate(global.get_current_time());
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
`);
	} else {
		capture("xdotool", ["windowactivate", "--sync", wid]);
	}
}

function windowMoveResize(wid, x, y, w, h) {
	if (windowBackend === "cinnamon") {
		cinnamonEval(`
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) 'missing';
else {
	try { w.unmaximize(3); } catch (e) {}
	w.move_resize_frame(true, ${x}, ${y}, ${w}, ${h});
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
`);
	} else {
		capture("wmctrl", ["-i", "-r", wid, "-b", "remove,maximized_vert,maximized_horz,fullscreen"]);
		capture("wmctrl", ["-i", "-r", wid, "-e", `0,${x},${y},${w},${h}`]);
	}
}

async function placeWindows(wid1, wid2) {
	let { ox, oy, sw, sh } = leftMonitorGeom();
	const gap = 0;
	const margin = 0;
	let w = Math.floor((sw - gap - 2 * margin) / 2);
	let h = Math.floor((w * 9) / 16);
	if (h > sh - 2 * margin) h = sh - 2 * margin;
	const y = oy + margin;
	let x1 = ox + margin;
	let x2 = ox + margin + w + gap;

	for (let pass = 0; pass < 3; pass++) {
		windowMoveResize(wid1, x1, y, w, h);
		windowMoveResize(wid2, x2, y, w, h);
		await sleep(350);

		const a = windowGeom(wid1);
		const b = windowGeom(wid2);
		if (a.w && b.w && a.x + a.w + gap <= b.x && a.y === b.y) break;

		x2 = a.x + a.w + gap;
		if (x2 + w > ox + sw - margin) {
			w = Math.max(200, Math.floor((ox + sw - margin - gap - a.x) / 2));
			x2 = a.x + a.w + gap;
			windowMoveResize(wid1, a.x, y, w, h);
			await sleep(200);
			const a2 = windowGeom(wid1);
			x2 = a2.x + a2.w + gap;
		}
		windowMoveResize(wid2, x2, y, w, h);
		await sleep(200);
	}

	const a = windowGeom(wid1);
	const b = windowGeom(wid2);
	log(
		`left monitor ${sw}x${sh}+${ox}+${oy}: P1 ${a.w}x${a.h}@${a.x},${a.y} P2 ${b.w}x${b.h}@${b.x},${b.y}`,
	);
}

function usage() {
	console.log("Usage: node scripts/launch.js [path/to/Mario Kart Wii.iso]");
	console.log("  ISO may also come from MKWII_ISO.");
	console.log("  Env:");
	console.log("    MKWII_DOLPHIN_DIR  user dirs root (default: tmp/dolphin-test)");
	console.log("    MKWII_VIDEO        Vulkan|OGL (default: Vulkan)");
	process.exit(1);
}

function requireFlatpak() {
	need("flatpak");
	if (capture("flatpak", ["info", FLATPAK_APP]).status !== 0) {
		die(`Dolphin Flatpak missing. Install: flatpak install flathub ${FLATPAK_APP}`);
	}
}

function launchDolphin(iso, user, label, mac, videoBackend) {
	const child = spawn(
		"flatpak",
		[
			"run",
			"--command=dolphin-emu",
			FLATPAK_APP,
			"-u",
			user,
			"-b",
			"-e",
			iso,
			"-v",
			videoBackend,
			"-C",
			"Dolphin.Core.EnableCheats=True",
			"-C",
			"Dolphin.Interface.RenderToMain=False",
			"-C",
			"Dolphin.Interface.ConfirmStop=False",
			"-C",
			"Dolphin.Analytics.Enabled=False",
			"-C",
			"Dolphin.Analytics.PermissionAsked=True",
			"-C",
			"Dolphin.DSP.Volume=0",
			"-C",
			`Dolphin.General.WirelessMac=${mac}`,
		],
		{ stdio: "ignore", detached: true },
	);
	child.unref();
	log(`started ${label} pid=${child.pid} user=${user}`);
	return child;
}

async function main() {
	const arg = process.argv[2];
	if (arg === "-h" || arg === "--help") usage();

	const iso = arg || process.env.MKWII_ISO || "";
	if (!iso) die("pass an ISO path or set MKWII_ISO");
	if (!existsSync(iso)) die(`ISO not found: ${iso}`);

	const videoBackend = process.env.MKWII_VIDEO || "Vulkan";
	requireFlatpak();
	initTilingBackends();

	const isoPath = realpathSync(iso);
	const u1 = join(USER_ROOT, "p1");
	const u2 = join(USER_ROOT, "p2");

	seedUser(u1, "00:17:ab:ca:ac:f1", "0", videoBackend);
	seedUser(u2, "00:17:ab:ca:ac:f2", "0", videoBackend);

	const children = [];
	const cleanup = () => {
		for (const c of children) {
			try {
				process.kill(c.pid);
			} catch {
				/* ignore */
			}
		}
	};
	process.on("SIGINT", () => {
		cleanup();
		process.exit(0);
	});
	process.on("SIGTERM", cleanup);

	log(`ISO: ${isoPath}`);
	log(`users: ${u1} , ${u2}`);
	children.push(
		launchDolphin(isoPath, u1, "P1", "00:17:ab:ca:ac:f1", videoBackend),
	);
	await sleep(200);
	children.push(
		launchDolphin(isoPath, u2, "P2", "00:17:ab:ca:ac:f2", videoBackend),
	);

	log("waiting for game windows...");
	const wids = await waitTwoWindows(90);
	await placeWindows(wids[0], wids[1]);
	setTimeout(() => placeWindows(wids[0], wids[1]), 2000);

	log("floating windows ready (left monitor, side by side, muted)");
	log("Ctrl+C stops both. Run: just auto");

	while (children.some((c) => {
		try {
			process.kill(c.pid, 0);
			return true;
		} catch {
			return false;
		}
	})) {
		await sleep(1000);
	}
}

main().catch((err) => die(err.message || String(err)));
