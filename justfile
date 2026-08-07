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

# Two muted batch Dolphin windows (NoSSL + cheats).
launch iso=iso:
	node scripts/launch.js "{{iso}}"

# Launch + tile on primary monitor + WFC menu automation (mkw-dwc must already be running).
test iso=iso:
	MKWII_TILE=1 node scripts/test.js "{{iso}}"

# Same as test, but do not move/tile windows (leave them where the desktop places them).
test2 iso=iso:
	node scripts/test.js "{{iso}}"
