#!/usr/bin/env bash
# Launch two Flatpak Dolphin Mario Kart Wii instances for local WFC testing.
# Hides the Dolphin settings UI (--batch). Shows two floating game windows.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FLATPAK_APP="${FLATPAK_APP:-org.DolphinEmu.dolphin-emu}"
USER_ROOT="${MKWII_DOLPHIN_DIR:-$REPO_ROOT/tmp/dolphin-test}"
ISO="${1:-${MKWII_ISO:-}}"
VIDEO_BACKEND="${MKWII_VIDEO:-Vulkan}"
# Seconds after both windows appear before sending the first A (auto only).
BOOT_WAIT="${MKWII_BOOT_WAIT:-10}"
# Leading A clicks to skip health warning / logos / title.
SKIP_AS="${MKWII_SKIP_AS:-18}"
# Pixels per IR nudge when opening Nintendo WFC from main menu.
IR_NUDGE="${MKWII_IR_NUDGE:-80}"
# Delay between skip-A presses (seconds).
A_INTERVAL="${MKWII_A_INTERVAL:-0.7}"

NOSSL_NAME='$NoSSL'
NOSSL_LINES=(
	'C0000000 0000000E'
	'3C004E80 60000020'
	'900F0000 3D808000'
	'618C3000 3C00017F'
	'6000CFFC 7C0903A6'
	'3D607474 616B7073'
	'800C0000 7C005800'
	'40A20034 394C0003'
	'392C0002 7D455378'
	'38600000 8C050001'
	'2C000000 38630001'
	'4082FFF4 8C0A0001'
	'9C090001 3463FFFF'
	'4082FFF4 398C0001'
	'4200FFC0 4E800020'
)

usage() {
	echo "Usage: $0 [path/to/Mario Kart Wii.iso]"
	echo "       $0 auto"
	echo "  ISO may also come from MKWII_ISO."
	echo "  auto  run menu automation on two already-open Mario Kart Wii windows"
	echo "  Env:"
	echo "    MKWII_DOLPHIN_DIR  user dirs root (default: tmp/dolphin-test)"
	echo "    MKWII_VIDEO        Vulkan|OGL (default: Vulkan)"
	echo "    MKWII_BOOT_WAIT    seconds before menu automation (default: 10 launch, 0 for auto)"
	echo "    MKWII_SKIP_AS      A clicks to skip health/logos/title (default: 18)"
	echo "    MKWII_A_INTERVAL   seconds between skip-A clicks (default: 0.7)"
	echo "    MKWII_AUTO         run menu automation after launch (default: 1)"
	exit 1
}

die() {
	echo "error: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

is_wayland() {
	[[ "${XDG_SESSION_TYPE:-}" == wayland ]]
}

is_cinnamon() {
	local desk="${XDG_CURRENT_DESKTOP:-}${DESKTOP_SESSION:-}"
	[[ "${desk}" == *[Cc]innamon* ]] || pgrep -x cinnamon >/dev/null 2>&1
}

init_backends() {
	# Flatpak Dolphin is XWayland. Automation uses xdotool mouse + Cinnamon focus.
	need xdotool
	if is_wayland; then
		if is_cinnamon; then
			WINDOW_BACKEND=cinnamon
			need gdbus
			need python3
		else
			die "Wayland automation needs Cinnamon (org.Cinnamon Eval). Use an X11 session or focus windows manually."
		fi
	else
		need wmctrl
		WINDOW_BACKEND=x11
	fi
}

# Run Cinnamon/Muffin JS via org.Cinnamon.Eval. Prints JSON on success.
cinnamon_eval() {
	python3 - "$@" <<'PY'
import ast
import json
import subprocess
import sys

script = sys.argv[1]
r = subprocess.run(
	[
		"gdbus",
		"call",
		"--session",
		"--dest",
		"org.Cinnamon",
		"--object-path",
		"/org/Cinnamon",
		"--method",
		"org.Cinnamon.Eval",
		script,
	],
	capture_output=True,
	text=True,
	check=True,
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
PY
}

write_nossl() {
	local ini="$1"
	mkdir -p "$(dirname "$ini")"
	# Do not put brackets in the cheat name. INI parsers treat [Fix94] as a
	# new section and the Gecko code never loads.
	{
		echo "[Core]"
		echo "EnableCheats = True"
		echo
		echo "[Gecko]"
		echo "${NOSSL_NAME}"
		printf '%s\n' "${NOSSL_LINES[@]}"
		echo "[Gecko_Enabled]"
		echo "${NOSSL_NAME}"
	} >"${ini}"
}

# Copy Flatpak Dolphin GCPad (Xbox etc.) and write basic keyboard Wiimote/GCPad.
# Dolphin XInput2 exposes keys on "Virtual core pointer", not "Virtual core keyboard".
controller_config_src() {
	local src="${MKWII_CONTROLLER_CONFIG:-${HOME}/.var/app/${FLATPAK_APP}/config/dolphin-emu}"
	if [[ -d "${src}" ]]; then
		echo "${src}"
	elif [[ -d "${src}/Config" ]]; then
		echo "${src}"
	fi
}

controller_ini_path() {
	local src="$1" name="$2"
	[[ -n "${src}" ]] || return 0
	if [[ -f "${src}/${name}" ]]; then
		echo "${src}/${name}"
	elif [[ -f "${src}/Config/${name}" ]]; then
		echo "${src}/Config/${name}"
	fi
}

# Arrow keys on XInput2 pointer read inverted for D-Pad; swap key names.
fix_wiimote_dpad_keys() {
	local ini="$1"
	[[ -f "${ini}" ]] || return 0
	python3 - "$ini" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
swap = {
	"D-Pad/Up": "Down",
	"D-Pad/Down": "Up",
	"D-Pad/Left": "Right",
	"D-Pad/Right": "Left",
}
lines = path.read_text().splitlines()
out, in_w1 = [], False
for line in lines:
	s = line.strip()
	if s.startswith("[") and s.endswith("]"):
		in_w1 = s == "[Wiimote1]"
		out.append(line)
		continue
	if in_w1 and "=" in line:
		key, _, val = line.partition("=")
		key, val = key.strip(), val.strip()
		if key in swap:
			out.append(f"{key} = {swap[key]}")
			continue
	out.append(line)
path.write_text("\n".join(out) + "\n")
PY
}

write_wiimote_pointer() {
	local ini="$1"
	mkdir -p "$(dirname "${ini}")"
	cat >"${ini}" <<'EOF'
[Wiimote1]
Device = XInput2/0/Virtual core pointer
Buttons/A = `Click 1`
Buttons/B = `Click 3`
Buttons/1 = `1`
Buttons/2 = `2`
Buttons/- = Q
Buttons/+ = E
Buttons/Home = Return
D-Pad/Up = Down
D-Pad/Down = Up
D-Pad/Left = Right
D-Pad/Right = Left
IR/Up = `Cursor Y-`
IR/Down = `Cursor Y+`
IR/Left = `Cursor X-`
IR/Right = `Cursor X+`
Extension = None
[Wiimote2]
Device = XInput2/0/Virtual core pointer
[Wiimote3]
Device = XInput2/0/Virtual core pointer
[Wiimote4]
Device = XInput2/0/Virtual core pointer
[BalanceBoard]
Device = XInput2/0/Virtual core pointer
EOF
}

write_gcpad_keyboard() {
	local ini="$1"
	mkdir -p "$(dirname "${ini}")"
	cat >"${ini}" <<'EOF'
[GCPad1]
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
D-Pad/Up = Down
D-Pad/Down = Up
D-Pad/Left = Right
D-Pad/Right = Left
[GCPad2]
Device = XInput2/0/Virtual core pointer
[GCPad3]
Device = XInput2/0/Virtual core pointer
[GCPad4]
Device = XInput2/0/Virtual core pointer
EOF
}

# OR keyboard binds onto an existing GCPad (Device stays on the gamepad).
or_gcpad_keyboard() {
	local ini="$1"
	[[ -f "${ini}" ]] || return 0
	python3 - "$ini" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
kb = "XInput2/0/Virtual core pointer"
binds = {
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
	"D-Pad/Up": "Down",
	"D-Pad/Down": "Up",
	"D-Pad/Left": "Right",
	"D-Pad/Right": "Left",
}

def expr(key: str) -> str:
	return f"(`{kb}:{key}`)"

lines = path.read_text().splitlines()
out, in_pad1, seen = [], False, set()
for line in lines:
	s = line.strip()
	if s.startswith("[") and s.endswith("]"):
		if in_pad1:
			for k, v in binds.items():
				if k not in seen:
					out.append(f"{k} = {expr(v)}")
			seen.clear()
		in_pad1 = s == "[GCPad1]"
		out.append(line)
		continue
	if not in_pad1 or "=" not in line:
		out.append(line)
		continue
	key, _, val = line.partition("=")
	key, val = key.strip(), val.strip()
	if key not in binds:
		out.append(line)
		continue
	seen.add(key)
	extra = expr(binds[key])
	if f"`{kb}:{binds[key]}`" in val:
		out.append(line)
	elif not val:
		out.append(f"{key} = {extra}")
	else:
		out.append(f"{key} = {val} | {extra}")
if in_pad1:
	for k, v in binds.items():
		if k not in seen:
			out.append(f"{k} = {expr(v)}")
path.write_text("\n".join(out) + "\n")
PY
}

seed_controllers() {
	local dir="$1"
	local src path ini
	mkdir -p "${dir}/Config"
	src="$(controller_config_src)"
	path="$(controller_ini_path "${src}" "GCPadNew.ini")"
	if [[ -n "${path}" ]]; then
		cp "${path}" "${dir}/Config/GCPadNew.ini"
		or_gcpad_keyboard "${dir}/Config/GCPadNew.ini"
	else
		write_gcpad_keyboard "${dir}/Config/GCPadNew.ini"
	fi
	path="$(controller_ini_path "${src}" "WiimoteNew.ini")"
	if [[ -n "${path}" ]]; then
		cp "${path}" "${dir}/Config/WiimoteNew.ini"
		fix_wiimote_dpad_keys "${dir}/Config/WiimoteNew.ini"
	else
		write_wiimote_pointer "${dir}/Config/WiimoteNew.ini"
	fi

	# GC ports so Xbox/keyboard GCPad works in Wii games (MKWii).
	ini="${dir}/Config/Dolphin.ini"
	if [[ -f "${ini}" ]]; then
		if grep -qE '^SIDevice0[[:space:]]*=' "${ini}"; then
			sed -i 's/^SIDevice0[[:space:]]*=.*/SIDevice0 = 6/' "${ini}"
		elif grep -qE '^\[Core\]' "${ini}"; then
			sed -i '/^\[Core\]/a SIDevice0 = 6' "${ini}"
		else
			printf '\n[Core]\nSIDevice0 = 6\n' >>"${ini}"
		fi
		if ! grep -qE '^BackgroundInput[[:space:]]*=' "${ini}"; then
			if grep -qE '^\[Input\]' "${ini}"; then
				sed -i '/^\[Input\]/a BackgroundInput = True' "${ini}"
			else
				printf '\n[Input]\nBackgroundInput = True\n' >>"${ini}"
			fi
		else
			sed -i 's/^BackgroundInput[[:space:]]*=.*/BackgroundInput = True/' "${ini}"
		fi
	fi
}

seed_user() {
	local dir="$1" mac="$2" volume="$3"
	mkdir -p "${dir}/Config" "${dir}/GameSettings"
	cat >"${dir}/Config/Dolphin.ini" <<EOF
[Analytics]
Enabled = False
PermissionAsked = True
[Core]
EnableCheats = True
GFXBackend = ${VIDEO_BACKEND}
SIDevice0 = 6
SIDevice1 = 6
[Interface]
ConfirmStop = False
RenderToMain = False
OnScreenDisplayMessages = False
PauseOnFocusLost = False
[Input]
BackgroundInput = True
[DSP]
Volume = ${volume}
[General]
WirelessMac = ${mac}
EOF
	seed_controllers "${dir}"
	for id in RMCE01 RMCJ01 RMCP01; do
		write_nossl "${dir}/GameSettings/${id}.ini"
	done
}

mkw_windows_cinnamon() {
	cinnamon_eval '
let wins = global.display.get_tab_list(0, null)
	.filter(w => w.get_title().includes("Mario Kart Wii"))
	.map(w => {
		let r = w.get_frame_rect();
		return {id: w.get_id(), title: w.get_title(), x: r.x, y: r.y, w: r.width, h: r.height};
	})
	.sort((a, b) => a.x - b.x)
	.slice(0, 2);
JSON.stringify(wins);
'
}

mkw_wids() {
	local wid name
	while IFS= read -r wid; do
		[[ -n "${wid}" ]] || continue
		name="$(xdotool getwindowname "${wid}" 2>/dev/null || true)"
		if [[ "${name}" == *"Mario Kart Wii"* ]]; then
			echo "${wid}"
		fi
	done < <(xdotool search --name 'Mario Kart Wii' 2>/dev/null || true)
}

# Two MKW windows, left-to-right by screen X (P1 then P2).
mkw_wids_ltr() {
	local -a wids=()
	local wid x
	mapfile -t wids < <(mkw_wids | sort -u)
	if ((${#wids[@]} < 2)); then
		return 1
	fi
	{
		for wid in "${wids[@]}"; do
			x="$(xdotool getwindowgeometry "${wid}" 2>/dev/null | awk '/Position:/{gsub(/,.*/,"",$2); print $2; exit}')"
			[[ -n "${x}" ]] || x=0
			printf '%s %s\n' "${x}" "${wid}"
		done
	} | sort -n -k1,1 | awk 'NR<=2 { print $2 }'
}

# Two MKW X11 window IDs, left-to-right by screen X (P1 then P2).
mkw_x11_ids_ltr() {
	mkw_wids_ltr
}

# Index of cinnamon meta id among LTR MKW windows (0-based), or empty.
cinnamon_mkw_index() {
	local target="$1"
	python3 -c '
import json, sys
target = sys.argv[1]
wins = json.loads(sys.stdin.read() or "[]")
for i, w in enumerate(wins):
    if str(w["id"]) == target:
        print(i)
        break
' "${target}" < <(mkw_windows_cinnamon)
}

# Resolve an automation window id to an X11 id for xdotool input.
# Cinnamon meta ids are mapped to X11 ids by left-to-right order.
x11_id_for() {
	local wid="$1"
	local idx
	local -a xids=()

	if [[ "${WINDOW_BACKEND}" != cinnamon ]]; then
		echo "${wid}"
		return 0
	fi

	mapfile -t xids < <(mkw_x11_ids_ltr) || true
	if ((${#xids[@]} < 2)); then
		die "need 2 X11 Mario Kart Wii windows for input (found ${#xids[@]})"
	fi
	idx="$(cinnamon_mkw_index "${wid}")"
	[[ -n "${idx}" ]] || die "cinnamon window ${wid} not in MKW pair"
	echo "${xids[${idx}]}"
}

mkw_pair_ids() {
	if [[ "${WINDOW_BACKEND}" == cinnamon ]]; then
		python3 -c '
import json, sys
wins = json.loads(sys.stdin.read() or "[]")
for w in wins:
    print(w["id"])
' < <(mkw_windows_cinnamon)
	else
		mkw_x11_ids_ltr
	fi
}

wait_two_windows() {
	local timeout_s="${1:-60}"
	local deadline=$((SECONDS + timeout_s))
	local -a wids=()
	while ((SECONDS < deadline)); do
		mapfile -t wids < <(mkw_pair_ids) || true
		if ((${#wids[@]} >= 2)); then
			echo "${wids[0]}"
			echo "${wids[1]}"
			return 0
		fi
		sleep 0.4
	done
	return 1
}

left_monitor_geom_cinnamon() {
	cinnamon_eval '
let m = Main.layoutManager.monitors.slice().sort((a, b) => a.x - b.x)[0];
JSON.stringify({x: m.x, y: m.y, w: m.width, h: m.height});
'
}

left_monitor_geom() {
	if [[ "${WINDOW_BACKEND}" == cinnamon ]]; then
		python3 -c '
import json, sys
m = json.loads(sys.stdin.read())
print(m["x"], m["y"], m["w"], m["h"])
' < <(left_monitor_geom_cinnamon)
		return 0
	fi
	# Print ox oy width height for the left-most connected output.
	xrandr --current 2>/dev/null | awk '
		/ connected/ {
			for (i = 1; i <= NF; i++) {
				if ($i ~ /^[0-9]+x[0-9]+\+[0-9]+\+[0-9]+$/) {
					split($i, a, /[x+]/)
					w = a[1] + 0
					h = a[2] + 0
					x = a[3] + 0
					y = a[4] + 0
					if (!seen || x < bestx) {
						bestx = x
						besty = y
						bestw = w
						besth = h
						seen = 1
					}
				}
			}
		}
		END {
			if (seen) print bestx, besty, bestw, besth
		}
	'
}

window_geom() {
	local wid="$1"
	if [[ "${WINDOW_BACKEND}" == cinnamon ]]; then
		python3 -c '
import json, sys
r = json.loads(sys.stdin.read())
print(r["x"], r["y"], r["w"], r["h"])
' < <(cinnamon_eval "
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) \"missing\";
else {
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
")
		return 0
	fi
	wmctrl -lG | awk -v id="${wid}" '
		strtonum($1) == strtonum(id) { print $3, $4, $5, $6; exit }
	'
}

window_activate() {
	local wid="$1"
	if [[ "${WINDOW_BACKEND}" == cinnamon ]]; then
		cinnamon_eval "
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) 'missing';
else {
	w.activate(global.get_current_time());
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
" >/dev/null
	else
		xdotool windowactivate --sync "${wid}"
	fi
	sleep 0.2
}

window_move_resize() {
	local wid="$1" x="$2" y="$3" win_w="$4" win_h="$5"
	if [[ "${WINDOW_BACKEND}" == cinnamon ]]; then
		cinnamon_eval "
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) 'missing';
else {
	try { w.unmaximize(3); } catch (e) {}
	w.move_resize_frame(true, ${x}, ${y}, ${win_w}, ${win_h});
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
" >/dev/null
	else
		wmctrl -i -r "${wid}" -b remove,maximized_vert,maximized_horz,fullscreen
		wmctrl -i -r "${wid}" -e "0,${x},${y},${win_w},${win_h}"
	fi
}

place_windows() {
	local wid1="$1" wid2="$2"
	local ox oy sw sh gap margin w h y x1 x2
	local ax ay aw ah bx by bw bh
	local pass

	ox=0
	oy=0
	if ! read -r ox oy sw sh < <(left_monitor_geom); then
		if [[ "${WINDOW_BACKEND}" == x11 ]]; then
			read -r sw sh < <(xdotool getdisplaygeometry)
		else
			die "could not read monitor geometry"
		fi
		ox=0
		oy=0
	fi

	# Equal tiles on the left monitor only, flush to edges.
	gap=0
	margin=0
	w=$(((sw - gap - 2 * margin) / 2))
	h=$((w * 9 / 16))
	if ((h > sh - 2 * margin)); then
		h=$((sh - 2 * margin))
	fi
	y=$((oy + margin))
	x1=$((ox + margin))
	x2=$((ox + margin + w + gap))

	for pass in 1 2 3; do
		window_move_resize "${wid1}" "${x1}" "${y}" "${w}" "${h}"
		window_move_resize "${wid2}" "${x2}" "${y}" "${w}" "${h}"
		sleep 0.35

		read -r ax ay aw ah < <(window_geom "${wid1}")
		read -r bx by bw bh < <(window_geom "${wid2}")
		[[ -n "${aw}" && -n "${bw}" ]] || continue

		# If frames still overlap or Y drifted, shove P2 flush right of P1.
		if ((ax + aw + gap > bx)) || ((ay != by)); then
			x2=$((ax + aw + gap))
			if ((x2 + w > ox + sw - margin)); then
				w=$(((ox + sw - margin - gap - ax) / 2))
				((w < 200)) && w=200
				x2=$((ax + aw + gap))
				window_move_resize "${wid1}" "${ax}" "${y}" "${w}" "${h}"
				sleep 0.2
				read -r ax ay aw ah < <(window_geom "${wid1}")
				x2=$((ax + aw + gap))
			fi
			window_move_resize "${wid2}" "${x2}" "${y}" "${w}" "${h}"
			sleep 0.2
		else
			break
		fi
	done

	read -r ax ay aw ah < <(window_geom "${wid1}")
	read -r bx by bw bh < <(window_geom "${wid2}")
	echo "left monitor ${sw}x${sh}+${ox}+${oy}: P1 ${aw}x${ah}@${ax},${ay} P2 ${bw}x${bh}@${bx},${by}"
}

# Cinnamon focus only.
focus_game() {
	local wid="$1"
	window_activate "${wid}"
	sleep 0.2
	x11_id_for "${wid}" >/dev/null
}

# Wiimote A = mouse left click (seeded profile). Dolphin is XWayland: use xdotool.
tap_a() {
	local wid="$1" xid x y
	focus_game "${wid}"
	xid="$(x11_id_for "${wid}")"
	eval "$(xdotool getwindowgeometry --shell "${xid}")"
	x=$((WIDTH / 2))
	y=$((HEIGHT / 2))
	xdotool mousemove --window "${xid}" "${x}" "${y}"
	sleep 0.05
	xdotool click --window "${xid}" 1
	sleep 0.08
}

# Nudge emulated IR via mouse (Cursor X/Y in seeded Wiimote profile).
ir_nudge() {
	local wid="$1" dx="$2" dy="$3" xid
	focus_game "${wid}"
	xid="$(x11_id_for "${wid}")"
	xdotool mousemove_relative --window "${xid}" "${dx}" "${dy}"
	sleep 0.15
}

# Drive both windows: skip health/logos/title -> license -> Nintendo WFC.
open_nintendo_wfc_both() {
	local wid1="$1" wid2="$2"
	local i nudge="${IR_NUDGE}"

	if ((BOOT_WAIT > 0)); then
		echo "waiting ${BOOT_WAIT}s before automation (MKWII_BOOT_WAIT)..."
		sleep "${BOOT_WAIT}"
	fi

	echo "menu navigate both: ${SKIP_AS} skip-A @ ${A_INTERVAL}s (mouse click)"
	for ((i = 0; i < SKIP_AS; i++)); do
		tap_a "${wid1}"
		tap_a "${wid2}"
		sleep "${A_INTERVAL}"
	done

	echo "license A (mouse click)"
	tap_a "${wid1}"
	tap_a "${wid2}"
	sleep 2.5

	echo "IR to Nintendo WFC + A (mouse nudge ${nudge}px)"
	for wid in "${wid1}" "${wid2}"; do
		ir_nudge "${wid}" 0 "${nudge}"
		ir_nudge "${wid}" 0 "${nudge}"
		ir_nudge "${wid}" "-${nudge}" 0
		ir_nudge "${wid}" "-${nudge}" 0
	done
	sleep 0.3
	tap_a "${wid1}"
	tap_a "${wid2}"
	sleep 0.8
	echo "Nintendo WFC open attempted on both windows"
}

# Menu automation on two already-open MKW windows.
run_auto() {
	init_backends
	local -a wids=()
	local -a xids=()

	mapfile -t wids < <(mkw_pair_ids) || true
	if ((${#wids[@]} < 2)); then
		die "need 2 open Mario Kart Wii windows (found ${#wids[@]})"
	fi
	mapfile -t xids < <(mkw_x11_ids_ltr) || true
	if ((${#xids[@]} < 2)); then
		die "need 2 X11 Mario Kart Wii windows for xdotool input (found ${#xids[@]})"
	fi
	echo "backends: window=${WINDOW_BACKEND} input=xdotool-mouse"
	echo "using open windows: P1=${wids[0]} (x11=${xids[0]}) P2=${wids[1]} (x11=${xids[1]})"
	open_nintendo_wfc_both "${wids[0]}" "${wids[1]}"
}

[[ "${1:-}" == "-h" || "${1:-}" == "--help" ]] && usage

if [[ "${1:-}" == "auto" || "${1:-}" == "--auto" || "${1:-}" == "wfc" || "${1:-}" == "--wfc" ]]; then
	# Already-open pair: start clicking immediately (no boot wait unless set).
	if [[ -z "${MKWII_BOOT_WAIT:-}" ]]; then
		BOOT_WAIT=0
	fi
	run_auto
	exit 0
fi

[[ -n "${ISO}" ]] || die "pass an ISO path or set MKWII_ISO"
[[ -f "${ISO}" ]] || die "ISO not found: ${ISO}"
need flatpak
init_backends
if ! flatpak info "${FLATPAK_APP}" >/dev/null 2>&1; then
	die "Dolphin Flatpak missing. Install: flatpak install flathub ${FLATPAK_APP}"
fi

ISO="$(readlink -f "${ISO}")"
U1="${USER_ROOT}/p1"
U2="${USER_ROOT}/p2"

# Distinct MACs so NAS/profile treat them as different consoles.
seed_user "${U1}" "00:17:ab:ca:ac:f1" "0"
seed_user "${U2}" "00:17:ab:ca:ac:f2" "0"

PIDS=()
cleanup() {
	local pid
	for pid in "${PIDS[@]:-}"; do
		kill "${pid}" 2>/dev/null || true
	done
	for pid in "${PIDS[@]:-}"; do
		wait "${pid}" 2>/dev/null || true
	done
}
trap cleanup EXIT INT TERM

launch() {
	local user="$1" label="$2" mac="$3"
	# Batch hides the Dolphin settings UI. Only the game render window stays.
	flatpak run --command=dolphin-emu "${FLATPAK_APP}" \
		-u "${user}" \
		-b \
		-e "${ISO}" \
		-v "${VIDEO_BACKEND}" \
		-C Dolphin.Core.EnableCheats=True \
		-C Dolphin.Interface.RenderToMain=False \
		-C Dolphin.Interface.ConfirmStop=False \
		-C Dolphin.Analytics.Enabled=False \
		-C Dolphin.Analytics.PermissionAsked=True \
		-C Dolphin.DSP.Volume=0 \
		-C "Dolphin.General.WirelessMac=${mac}" \
		>/dev/null 2>&1 &
	PIDS+=("$!")
	echo "started ${label} pid=$! user=${user}"
}

echo "ISO: ${ISO}"
echo "users: ${U1} , ${U2}"
launch "${U1}" "P1" "00:17:ab:ca:ac:f1"
# Short gap only so both stay close on boot / title timing.
sleep 0.2
launch "${U2}" "P2" "00:17:ab:ca:ac:f2"

echo "waiting for game windows..."
mapfile -t WIDS < <(wait_two_windows 90) || die "expected 2 Mario Kart Wii windows"
place_windows "${WIDS[0]}" "${WIDS[1]}"
# Dolphin often resizes once the game starts rendering. Re-tile after settle.
(
	sleep 2
	place_windows "${WIDS[0]}" "${WIDS[1]}"
) &
echo "floating windows ready (left monitor, side by side, muted)"
if [[ "${MKWII_AUTO:-1}" != 0 ]]; then
	echo "running menu automation (mouse IR + click)..."
	open_nintendo_wfc_both "${WIDS[0]}" "${WIDS[1]}"
else
	echo "menu automation skipped (MKWII_AUTO=0). Run: just auto"
fi
echo "Ctrl+C stops both."

while kill -0 "${PIDS[0]}" 2>/dev/null || kill -0 "${PIDS[1]}" 2>/dev/null; do
	sleep 1
done
