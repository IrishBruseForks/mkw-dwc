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
BOOT_WAIT="${MKWII_BOOT_WAIT:-40}"
# Extra leading A presses to skip logos / opening movie (0 if boot wait is long enough).
SKIP_AS="${MKWII_SKIP_AS:-1}"

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
	echo "    MKWII_DOLPHIN_DIR       user dirs root (default: tmp/dolphin-test)"
	echo "    MKWII_CONTROLLER_CONFIG source for Wiimote/GCPad INIs (Flatpak Dolphin)"
	echo "    MKWII_VIDEO             Vulkan|OGL (default: Vulkan)"
	echo "    MKWII_BOOT_WAIT         seconds before menu keys (default: 40 launch, 0 for auto)"
	echo "    MKWII_SKIP_AS           extra A presses before title (default: 1)"
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
	if is_wayland; then
		need ydotool
		INPUT_BACKEND=ydotool
		if is_cinnamon; then
			WINDOW_BACKEND=cinnamon
			need gdbus
			need python3
		else
			die "Wayland automation needs Cinnamon (org.Cinnamon Eval). Use an X11 session or focus windows manually."
		fi
	else
		need xdotool
		need wmctrl
		INPUT_BACKEND=xdotool
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

controller_config_src() {
	local src="${MKWII_CONTROLLER_CONFIG:-${HOME}/.var/app/${FLATPAK_APP}/config/dolphin-emu}"
	if [[ ! -d "${src}" ]]; then
		die "controller config dir not found: ${src} (set MKWII_CONTROLLER_CONFIG)"
	fi
	echo "${src}"
}

controller_ini_path() {
	local src="$1" name="$2"
	if [[ -f "${src}/${name}" ]]; then
		echo "${src}/${name}"
	elif [[ -f "${src}/Config/${name}" ]]; then
		echo "${src}/Config/${name}"
	fi
}

# Keep mouse left click on Wiimote A for automation, even when layering
# controller bindings from the user's Dolphin profile.
ensure_wiimote_click_a() {
	local ini="$1"
	[[ -f "${ini}" ]] || return 0
	if grep -qE 'Buttons/A =.*`Click 1`' "${ini}"; then
		return 0
	fi
	awk '
		/^Buttons\/A = / {
			line = $0
			sub(/^Buttons\/A = /, "", line)
			if (line !~ /`Click 1`/) {
				print "Buttons/A = " line " | `Click 1`"
				next
			}
		}
		{ print }
	' "${ini}" >"${ini}.tmp" && mv "${ini}.tmp" "${ini}"
}

# Copy the user's controller profile into an isolated test user dir.
seed_controllers() {
	local dir="$1"
	local src path name
	src="$(controller_config_src)"
	mkdir -p "${dir}/Config"
	for name in WiimoteNew.ini GCPadNew.ini GCKeyNew.ini; do
		path="$(controller_ini_path "${src}" "${name}")"
		if [[ -n "${path}" ]]; then
			cp "${path}" "${dir}/Config/${name}"
		fi
	done
	ensure_wiimote_click_a "${dir}/Config/WiimoteNew.ini"
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
[Interface]
ConfirmStop = False
RenderToMain = False
OnScreenDisplayMessages = False
PauseOnFocusLost = False
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

mkw_pair_ids() {
	if [[ "${WINDOW_BACKEND}" == cinnamon ]]; then
		python3 -c '
import json, sys
wins = json.loads(sys.stdin.read() or "[]")
for w in wins:
    print(w["id"])
' < <(mkw_windows_cinnamon)
	else
		mkw_wids_ltr
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

	# Equal tiles on the left monitor only. Keep a real gutter so frame
	# chrome cannot overlap, and use the same Y for both windows.
	gap=32
	margin=12
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

input_key() {
	local key="$1"
	if [[ "${INPUT_BACKEND}" == ydotool ]]; then
		case "${key}" in
		Down) ydotool key 108:1 108:0 ;;
		Up) ydotool key 103:1 103:0 ;;
		Left) ydotool key 105:1 105:0 ;;
		Right) ydotool key 106:1 106:0 ;;
		*) die "unsupported ydotool key: ${key}" ;;
		esac
	else
		xdotool key --clearmodifiers "${key}"
	fi
}

# Wiimote A is mouse left click in the seeded profile.
tap_a() {
	local wid="$1"
	local cx cy
	window_activate "${wid}"
	if [[ "${INPUT_BACKEND}" == ydotool ]]; then
		read -r cx cy < <(python3 -c '
import json, sys
r = json.loads(sys.stdin.read())
print(r["x"] + r["w"] // 2, r["y"] + r["h"] // 2)
' < <(cinnamon_eval "
let w = global.display.get_tab_list(0, null).find(x => x.get_id() === ${wid});
if (!w) 'missing';
else {
	let r = w.get_frame_rect();
	JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
}
"))
		ydotool mousemove "${cx}" "${cy}"
		sleep 0.05
		ydotool click 1
	else
		local x y
		eval "$(xdotool getwindowgeometry --shell "${wid}")"
		x=$((WIDTH / 2))
		y=$((HEIGHT / 2))
		xdotool mousemove --window "${wid}" "${x}" "${y}"
		sleep 0.05
		xdotool click --window "${wid}" 1
	fi
}

# Send one emulated Wiimote button/key to a focused Dolphin window.
tap() {
	local wid="$1" key="$2"
	window_activate "${wid}"
	sleep 0.1
	if [[ "${INPUT_BACKEND}" == ydotool ]]; then
		input_key "${key}"
	else
		xdotool key --window "${wid}" --clearmodifiers "${key}"
	fi
}

# Title -> license -> main menu -> Nintendo WFC -> 1 Player.
# Assumes an existing license (rksys.dat). Main menu order is Single Player,
# Multiplayer, Nintendo WFC. 1 Player is the default under Nintendo WFC.
open_nintendo_wfc() {
	local wid="$1" label="$2"
	local i

	echo "menu navigate ${label} (wid=${wid})"
	window_activate "${wid}"

	for ((i = 0; i < SKIP_AS; i++)); do
		tap_a "${wid}"
		sleep 2
	done

	# Title "Press A Button"
	tap_a "${wid}"
	sleep 3

	# License select (first / only license)
	tap_a "${wid}"
	sleep 5

	# Main menu: Single Player -> Multiplayer -> Nintendo WFC
	tap "${wid}" Down
	sleep 0.4
	tap "${wid}" Down
	sleep 0.9

	# Nintendo WFC: 1 Player is highlighted by default
	tap_a "${wid}"
	sleep 1
	echo "menu navigate ${label}: sent Nintendo WFC 1 Player"
}

open_nintendo_wfc_both() {
	local wid1="$1" wid2="$2"

	if ((BOOT_WAIT > 0)); then
		echo "waiting ${BOOT_WAIT}s for title screens (MKWII_BOOT_WAIT)..."
		sleep "${BOOT_WAIT}"
	fi
	open_nintendo_wfc "${wid1}" "P1"
	open_nintendo_wfc "${wid2}" "P2"
	echo "Nintendo WFC open attempted on both windows"
}

# Menu automation only: find two open MKW windows and drive Nintendo WFC.
run_auto() {
	init_backends
	local -a wids=()

	# Keep on-disk bindings ready for the next launch (running Dolphin will
	# not reload mid-session).
	seed_controllers "${USER_ROOT}/p1"
	seed_controllers "${USER_ROOT}/p2"

	mapfile -t wids < <(mkw_pair_ids) || true
	if ((${#wids[@]} < 2)); then
		die "need 2 open Mario Kart Wii windows (found ${#wids[@]})"
	fi
	echo "using open windows: P1=${wids[0]} P2=${wids[1]}"
	open_nintendo_wfc_both "${wids[0]}" "${wids[1]}"
}

[[ "${1:-}" == "-h" || "${1:-}" == "--help" ]] && usage

if [[ "${1:-}" == "auto" || "${1:-}" == "--auto" || "${1:-}" == "wfc" || "${1:-}" == "--wfc" ]]; then
	# Already-open pair: no boot wait unless the user sets MKWII_BOOT_WAIT.
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
sleep 1
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
echo "Ctrl+C stops both. Run 'just auto' to open Nintendo WFC menus."

while kill -0 "${PIDS[0]}" 2>/dev/null || kill -0 "${PIDS[1]}" 2>/dev/null; do
	sleep 1
done
