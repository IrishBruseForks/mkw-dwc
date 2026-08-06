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

# Two muted batch Dolphin windows (NoSSL + cheats), then mouse menu automation.
test iso=iso:
	#!/usr/bin/env bash
	set -euo pipefail
	iso="{{iso}}"
	if [[ -z "${iso}" ]]; then
		echo "error: set MKWII_ISO or pass a path: just test /path/to/RMCE01.iso" >&2
		exit 1
	fi
	if [[ ! -f "${iso}" ]]; then
		echo "error: ISO not found: ${iso}" >&2
		exit 1
	fi
	exec ./scripts/test-mkwii.sh "${iso}"

# Run Nintendo WFC menu automation on two already-open Mario Kart Wii windows.
auto:
	./scripts/test-mkwii.sh auto
