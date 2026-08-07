#!/usr/bin/env node
// Wipe rksys.dat and seed distinct DWC_AUTHDATA for dual Dolphin users.
// Avoids error 60000 (stale Friend Code) and 61020 (shared NAS userid).
import {
	existsSync,
	mkdirSync,
	readdirSync,
	renameSync,
	rmSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(__dirname, "..");
const FLATPAK_APP = "org.DolphinEmu.dolphin-emu";

function log(...args) {
	console.log(...args);
}

// DWC_AUTHDATA is 32 bytes; userid is a big-endian u64 at offset 0.
export function writeAuthdata(path, userid) {
	mkdirSync(dirname(path), { recursive: true });
	const buf = Buffer.alloc(32);
	buf.writeBigUInt64BE(BigInt(userid), 0);
	writeFileSync(path, buf);
}

export function wipeRksys(userDir) {
	const titleRoot = join(userDir, "Wii", "title");
	if (!existsSync(titleRoot)) return 0;
	let n = 0;
	const walk = (dir) => {
		for (const name of readdirSync(dir)) {
			const p = join(dir, name);
			const st = statSync(p);
			if (st.isDirectory()) {
				walk(p);
				continue;
			}
			if (name === "rksys.dat") {
				rmSync(p);
				n += 1;
			}
		}
	};
	walk(titleRoot);
	return n;
}

export function flatpakUserDir() {
	return join(
		process.env.HOME || "",
		`.var/app/${FLATPAK_APP}/data/dolphin-emu`,
	);
}

/** Wipe Flatpak default-user MKW saves (error 60000 if FC/PID is stale). */
export function wipeFlatpakRksys() {
	const dir = flatpakUserDir();
	if (!existsSync(dir)) return 0;
	const wiped = wipeRksys(dir);
	if (wiped > 0) {
		log(`wiped ${wiped} rksys.dat under Flatpak Dolphin ${dir} (avoid error 60000)`);
	}
	return wiped;
}

export function stashFlatpakAuthdata() {
	const path = join(flatpakUserDir(), "Wii", "shared2", "DWC_AUTHDATA");
	if (!existsSync(path)) return;
	const bak = `${path}.mkw-dwc.bak`;
	try {
		if (existsSync(bak)) rmSync(bak);
		renameSync(path, bak);
		log(`moved Flatpak DWC_AUTHDATA aside -> ${bak}`);
	} catch (err) {
		log(`warn: could not stash Flatpak AUTHDATA: ${err.message || err}`);
	}
}

export function seedIdentities(userDir, userid) {
	const wiped = wipeRksys(userDir);
	if (wiped > 0) {
		log(`wiped ${wiped} rksys.dat under ${userDir} (avoid error 60000)`);
	}
	writeAuthdata(join(userDir, "Wii", "shared2", "DWC_AUTHDATA"), userid);
	log(`seeded DWC_AUTHDATA userid=${userid} in ${userDir}`);
}

/** Seed p1 (userid 2) and p2 (userid 3) under userRoot, and wipe Flatpak FC. */
export function seedDualUsers(userRoot) {
	wipeFlatpakRksys();
	stashFlatpakAuthdata();
	const u1 = join(userRoot, "p1");
	const u2 = join(userRoot, "p2");
	mkdirSync(u1, { recursive: true });
	mkdirSync(u2, { recursive: true });
	seedIdentities(u1, 2);
	seedIdentities(u2, 3);
}

function main() {
	const userRoot =
		process.argv[2] ||
		process.env.MKWII_DOLPHIN_DIR ||
		join(REPO_ROOT, "tmp/dolphin-test");
	seedDualUsers(userRoot);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	main();
}
