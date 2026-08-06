#!/usr/bin/env node
// Local Dolphin + mkw-dwc helpers. Usage: scripts/local.js <command> [args]
"use strict";

const { spawnSync } = require("node:child_process");
const {
	accessSync,
	appendFileSync,
	constants,
	copyFileSync,
	existsSync,
	mkdirSync,
	readFileSync,
	writeFileSync,
} = require("node:fs");
const { dirname, join } = require("node:path");

const REPO_ROOT = join(__dirname, "..");
const BIN = join(REPO_ROOT, "mkw-dwc");
const CONFIG = join(REPO_ROOT, "mkw-dwc.ini");
const FLATPAK_APP = "org.DolphinEmu.dolphin-emu";
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

function log(...args) {
	console.log(...args);
}

function die(msg) {
	console.error(`error: ${msg}`);
	process.exit(1);
}

function hasCmd(cmd) {
	return spawnSync("sh", ["-c", `command -v -- ${JSON.stringify(cmd)}`], {
		stdio: "ignore",
	}).status === 0;
}

function need(cmd) {
	if (!hasCmd(cmd)) die(`missing command: ${cmd}`);
}

function run(cmd, args, opts = {}) {
	const r = spawnSync(cmd, args, { stdio: "inherit", ...opts });
	if (r.error) die(r.error.message);
	if (r.status !== 0) process.exit(r.status ?? 1);
}

function capture(cmd, args) {
	return spawnSync(cmd, args, {
		encoding: "utf8",
		stdio: ["ignore", "pipe", "pipe"],
	});
}

function isRoot() {
	return typeof process.getuid === "function" && process.getuid() === 0;
}

function canBind80() {
	try {
		accessSync(BIN, constants.R_OK);
		if (hasCmd("getcap")) {
			const r = capture("getcap", [BIN]);
			return (r.stdout || "").includes("cap_net_bind_service");
		}
	} catch {
		/* fall through */
	}
	return isRoot();
}

function requireFlatpakDolphin() {
	need("flatpak");
	if (capture("flatpak", ["info", FLATPAK_APP]).status !== 0) {
		die(`Dolphin not found. Install: flatpak install flathub ${FLATPAK_APP}`);
	}
}

function dolphinDataDir() {
	return join(
		process.env.HOME,
		`.var/app/${FLATPAK_APP}/data/dolphin-emu`,
	);
}

function dolphinConfigDir() {
	return join(
		process.env.HOME,
		`.var/app/${FLATPAK_APP}/config/dolphin-emu`,
	);
}

function cmdBuild() {
	need("go");
	log(`building ${BIN}`);
	run("go", ["build", "-o", "mkw-dwc", "."], { cwd: REPO_ROOT });
	log(`done: ${BIN}`);
}

function cmdGrantBind() {
	need("setcap");
	try {
		accessSync(BIN, constants.X_OK);
	} catch {
		die("binary not found, run: scripts/local.js build");
	}
	if (!isRoot()) {
		log(`needs root once to set capabilities on ${BIN}`);
		run("sudo", ["-E", process.execPath, __filename, "grant-bind"]);
		return;
	}
	run("setcap", ["cap_net_bind_service=+ep", BIN]);
	run("getcap", [BIN]);
	log("mkw-dwc can bind port 80 without sudo");
}

function stamp() {
	const d = new Date();
	const p = (n) => String(n).padStart(2, "0");
	return (
		`${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}` +
		`${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
	);
}

function writeGeckoIni(iniPath) {
	const gecko = [
		"[Gecko]",
		NOSSL_NAME,
		...NOSSL_LINES,
		"",
		"[Gecko_Enabled]",
		NOSSL_NAME,
		"",
	].join("\n");

	if (existsSync(iniPath)) {
		const existing = readFileSync(iniPath, "utf8");
		if (existing.includes(NOSSL_NAME)) {
			log(`already configured: ${iniPath}`);
			return;
		}
		copyFileSync(iniPath, `${iniPath}.bak.${stamp()}`);
		appendFileSync(iniPath, `\n${gecko}`);
		log(`appended NoSSL to ${iniPath}`);
	} else {
		writeFileSync(iniPath, gecko);
		log(`created ${iniPath}`);
	}
}

function enableCheats(iniPath) {
	if (!existsSync(iniPath)) {
		mkdirSync(dirname(iniPath), { recursive: true });
		writeFileSync(iniPath, "[Core]\nEnableCheats = True\n");
		log(`created ${iniPath} (EnableCheats = True)`);
		return;
	}

	let text = readFileSync(iniPath, "utf8");
	if (/^\s*EnableCheats\s*=\s*True/m.test(text)) {
		log(`cheats already enabled: ${iniPath}`);
		return;
	}

	if (/^EnableCheats\s*=/m.test(text)) {
		text = text.replace(/^\s*EnableCheats.*/m, "EnableCheats = True");
	} else if (/^\[Core\]/m.test(text)) {
		text = text.replace(/^\[Core\]/m, "[Core]\nEnableCheats = True");
	} else {
		text += "\n[Core]\nEnableCheats = True\n";
	}
	writeFileSync(iniPath, text);
	log(`set EnableCheats = True in ${iniPath}`);
}

function cmdDolphin() {
	requireFlatpakDolphin();
	const dataDir = dolphinDataDir();
	const configDir = dolphinConfigDir();
	mkdirSync(join(dataDir, "GameSettings"), { recursive: true });
	for (const gameId of GAME_IDS) {
		writeGeckoIni(join(dataDir, "GameSettings", `${gameId}.ini`));
	}
	enableCheats(join(configDir, "Dolphin.ini"));
	log(`Flatpak Dolphin: flatpak run ${FLATPAK_APP}`);
	log("In Dolphin: Properties -> Gecko Codes, confirm NoSSL is checked");
}

function cmdRun() {
	try {
		accessSync(BIN, constants.X_OK);
	} catch {
		die("binary not found, run: scripts/local.js build");
	}
	const args = ["--config", CONFIG];
	if (canBind80()) {
		log("starting mkw-dwc (NAS on port 80, no sudo)");
		run(BIN, args, { cwd: REPO_ROOT });
		return;
	}
	log("port 80 needs CAP_NET_BIND_SERVICE or root");
	log("one-time fix: scripts/local.js grant-bind");
	log("starting mkw-dwc with sudo");
	run("sudo", [BIN, ...args], { cwd: REPO_ROOT });
}

async function check(label, url) {
	let res;
	try {
		res = await fetch(url);
	} catch (err) {
		die(`${label} failed: ${url} (${err.message})`);
	}
	const body = (await res.text()).trim();
	if (!res.ok) die(`${label} failed: ${url} (HTTP ${res.status})`);
	if (body !== "ok") die(`${label} unexpected body (want ok): ${body}`);
	log(`${label}: ok`);
}

async function cmdHealth() {
	await check("NAS :80", "http://127.0.0.1/");
}

async function cmdSetup(args) {
	let runAfter = false;
	for (const arg of args) {
		if (arg === "--run") runAfter = true;
	}
	cmdBuild();
	cmdGrantBind();
	cmdDolphin();
	log("");
	log("setup complete");
	log("  1. just hosts                      install /etc/hosts aliases");
	log("  2. scripts/local.js run            start mkw-dwc");
	log("  3. scripts/local.js health         verify (while server runs)");
	log("  4. just test /path/to/MKWii.iso   two NoSSL Dolphin clients");
	log("  5. MKWii -> Nintendo WFC -> Wi-Fi Connection");
	log("");
	log("undo hosts: just hosts-uninstall");
	if (runAfter) cmdRun();
}

function usage() {
	console.log(`Local Dolphin + mkw-dwc helpers (Linux).

Usage: scripts/local.js <command> [args]

Commands:
  setup [--run]       build, grant-bind, dolphin NoSSL
  build               go build -o mkw-dwc
  grant-bind          one-time setcap for port 80 without sudo
  dolphin             NoSSL Gecko + EnableCheats for Flatpak Dolphin
  run                 start mkw-dwc (NAS on port 80)
  health              fetch NAS on :80 (server must be running)

Hosts aliases: just hosts / just hosts-uninstall

Quick start:
  just hosts
  scripts/local.js setup
  scripts/local.js run
  # other terminal:
  scripts/local.js health
  just test /path/to/MKWii.iso`);
}

async function main() {
	const [cmd, ...args] = process.argv.slice(2);
	switch (cmd) {
		case "setup":
			await cmdSetup(args);
			break;
		case "build":
			cmdBuild();
			break;
		case "grant-bind":
			cmdGrantBind();
			break;
		case "dolphin":
			cmdDolphin();
			break;
		case "run":
			cmdRun();
			break;
		case "health":
			await cmdHealth();
			break;
		case "-h":
		case "--help":
		case "help":
		case undefined:
			usage();
			break;
		default:
			die(`unknown command: ${cmd} (try: scripts/local.js help)`);
	}
}

main().catch((err) => die(err.message || String(err)));
