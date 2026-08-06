# Local Dolphin / Mario Kart Wii helpers

flatpak_app := "org.DolphinEmu.dolphin-emu"
iso := env_var_or_default("MKWII_ISO", "")

# Install Nintendo WFC host aliases in /etc/hosts (needs sudo)
hosts ip="127.0.0.1":
	./scripts/hosts.sh {{ip}}

# Remove managed host aliases from /etc/hosts (needs sudo)
hosts-uninstall:
	./scripts/hosts.sh uninstall

run:
	sudo go run . --config mkw-dwc.ini

# Two muted batch Dolphin windows (NoSSL + cheats), tiled side by side.
launch iso=iso:
	#!/usr/bin/env bash
	set -euo pipefail
	iso="{{iso}}"
	if [[ -z "${iso}" ]]; then
		echo "error: set MKWII_ISO or pass a path: just launch /path/to/RMCE01.iso" >&2
		exit 1
	fi
	if [[ ! -f "${iso}" ]]; then
		echo "error: ISO not found: ${iso}" >&2
		exit 1
	fi
	exec node scripts/launch.js "${iso}"

# Menu automation on two already-open Mario Kart Wii windows.
auto:
	node scripts/automate.js

# mkw-dwc (bg), two Dolphin clients (bg), then WFC menu automation.
up iso=iso:
	#!/usr/bin/env bash
	set -euo pipefail
	iso="{{iso}}"
	if [[ -z "${iso}" ]]; then
		echo "error: set MKWII_ISO or pass a path: just up /path/to/RMCE01.iso" >&2
		exit 1
	fi
	if [[ ! -f "${iso}" ]]; then
		echo "error: ISO not found: ${iso}" >&2
		exit 1
	fi

	root="{{justfile_directory()}}"
	pids=()
	cleanup() {
		local pid
		for pid in "${pids[@]:-}"; do
			kill "${pid}" 2>/dev/null || true
		done
	}
	trap cleanup EXIT INT TERM

	echo "starting mkw-dwc (background)..."
	just run &
	pids+=("$!")

	echo "waiting for NAS on :80..."
	for ((i = 0; i < 60; i++)); do
		if curl -sf -H "Host: naswii.nintendowifi.net" http://127.0.0.1/ >/dev/null; then
			break
		fi
		if ((i == 59)); then
			echo "error: mkw-dwc did not become ready on port 80" >&2
			exit 1
		fi
		sleep 1
	done

	echo "launching Dolphin clients (background)..."
	just launch "${iso}" &
	pids+=("$!")

	echo "waiting for two Mario Kart Wii windows, then running menu automation..."
	MKWII_WINDOW_WAIT=120 just auto

	echo "automation done; Ctrl+C stops server and Dolphin clients"
	wait
