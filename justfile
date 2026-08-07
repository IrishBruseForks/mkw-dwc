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
	sudo killall mkw-dwc || echo ok
	sudo go run . --config mkw-dwc.ini

# Two Dolphin windows (AUTHDATA/rksys wipe+seed). No tiling or menu automation.
open iso=iso:
	./scripts/open.sh "{{iso}}"

# Two muted batch Dolphin windows (NoSSL + cheats + AUTHDATA/rksys wipe+seed).
launch iso=iso:
	node scripts/launch.js "{{iso}}"

# Launch + tile on primary monitor + WFC menu automation (mkw-dwc must already be running).
# Wipes/seeds via launch.js.
test iso=iso:
	MKWII_TILE=1 node scripts/test.js "{{iso}}"

# Same as test, but do not move/tile windows (leave them where the desktop places them).
# Wipes/seeds via launch.js.
test2 iso=iso:
	node scripts/test.js "{{iso}}"
