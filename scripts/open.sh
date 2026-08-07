#!/usr/bin/env bash
# Open two Dolphin MKWii windows. Seeds AUTHDATA/rksys, no tiling or menu automation.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FLATPAK_APP="${MKWII_FLATPAK:-org.DolphinEmu.dolphin-emu}"
USER_ROOT="${MKWII_DOLPHIN_DIR:-$REPO_ROOT/tmp/dolphin-test}"
ISO="${1:-${MKWII_ISO:-}}"
VIDEO="${MKWII_VIDEO:-Vulkan}"

if [[ -z "$ISO" ]]; then
	echo "usage: $0 /path/to/Mario Kart Wii.iso" >&2
	echo "  or set MKWII_ISO" >&2
	exit 1
fi
if [[ ! -f "$ISO" ]]; then
	echo "error: ISO not found: $ISO" >&2
	exit 1
fi

U1="$USER_ROOT/p1"
U2="$USER_ROOT/p2"
mkdir -p "$U1" "$U2"

# Fresh licenses + distinct NAS userids (same as just launch / test).
node "$REPO_ROOT/scripts/seed-identities.js" "$USER_ROOT"

run_one() {
	local label=$1 user=$2 mac=$3
	flatpak run --command=dolphin-emu "$FLATPAK_APP" \
		-u "$user" -b -e "$ISO" -v "$VIDEO" \
		-C Dolphin.Core.EnableCheats=True \
		-C Dolphin.Interface.RenderToMain=False \
		-C Dolphin.Interface.ConfirmStop=False \
		-C Dolphin.DSP.Volume=0 \
		-C "Dolphin.General.WirelessMac=$mac" \
		>/dev/null 2>&1 &
	echo "started $label pid=$! user=$user"
}

echo "ISO: $ISO"
run_one P1 "$U1" "00:17:ab:ca:ac:f1"
sleep 0.2
run_one P2 "$U2" "00:17:ab:ca:ac:f2"
echo "two Dolphin windows launching (no tiling, no automation)"
