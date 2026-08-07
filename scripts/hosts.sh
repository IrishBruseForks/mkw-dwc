#!/usr/bin/env bash
# Install or remove Nintendo WFC host aliases in /etc/hosts (needs sudo).
set -euo pipefail

HOSTS_FILE="/etc/hosts"
BEGIN='# <!-- mkw-dwc -->'
END='# <!-- /mkw-dwc -->'

NAMES=(
	naswii.nintendowifi.net
	nas.nintendowifi.net
	dls1.nintendowifi.net
	conntest.nintendowifi.net
	gamespy.com
	mariokartwii.available.gs.nintendowifi.net
	mariokartwii.master.gs.nintendowifi.net
	mariokartwii.ms19.gs.nintendowifi.net
	mariokartwii.natneg1.gs.nintendowifi.net
	mariokartwii.natneg2.gs.nintendowifi.net
	mariokartwii.natneg3.gs.nintendowifi.net
	gpcm.gs.nintendowifi.net
	gpsp.gs.nintendowifi.net
	sake.gs.nintendowifi.net
	secure.sake.gs.nintendowifi.net
	peerchat.gs.nintendowifi.net
	gamestats.gs.nintendowifi.net
	gamestats2.gs.nintendowifi.net
)

usage() {
	echo "Usage: $0 [ip] | uninstall | block-dead | unblock-dead"
	echo "  [ip]           add aliases (default: 127.0.0.1)"
	echo "  uninstall      remove managed aliases"
	echo "  block-dead     REJECT outbound TCP to dead GameSpy 69.10.0.0/16 (fast fail if DNS was cached)"
	echo "  unblock-dead   remove that REJECT rule"
	exit 1
}

need_root() {
	if [[ "$(id -u)" -ne 0 ]]; then
		exec sudo "$0" "$@"
	fi
}

# Match current region or older marker styles.
is_begin() {
	[[ "$1" == "${BEGIN}" ]] || [[ "$1" == \#\ mkw-dwc\ local\ testing* ]]
}

is_end() {
	[[ "$1" == "${END}" ]] || [[ "$1" == "# end mkw-dwc" ]]
}

block_present() {
	grep -qF "${BEGIN}" "${HOSTS_FILE}" 2>/dev/null \
		|| grep -q '^# mkw-dwc local testing' "${HOSTS_FILE}" 2>/dev/null
}

install_hosts() {
	local ip="${1:-127.0.0.1}" name
	if block_present; then
		echo "hosts block already present in ${HOSTS_FILE}"
		echo "run: $0 uninstall"
		exit 0
	fi
	{
		echo "${BEGIN}"
		for name in "${NAMES[@]}"; do
			echo "${ip} ${name}"
		done
		echo "${END}"
	} >>"${HOSTS_FILE}"
	echo "installed host aliases -> ${ip}"
	block_dead_gamespy || true
}

uninstall_hosts() {
	if ! block_present; then
		echo "no mkw-dwc hosts block in ${HOSTS_FILE}"
		exit 0
	fi
	local tmp line skip=0
	tmp="$(mktemp)"
	while IFS= read -r line || [[ -n "${line}" ]]; do
		if is_begin "${line}"; then
			skip=1
			continue
		fi
		if is_end "${line}"; then
			skip=0
			continue
		fi
		if [[ "${skip}" -eq 0 ]]; then
			printf '%s\n' "${line}"
		fi
	done <"${HOSTS_FILE}" >"${tmp}"
	cp "${tmp}" "${HOSTS_FILE}"
	rm -f "${tmp}"
	echo "removed host aliases from ${HOSTS_FILE}"
}

# Old GameSpy anycast (69.10.0.0/16) is blackholed. Cached A records still cause
# multi-second TCP SYN timeouts during MKWii WFC connect (SAKE/peerchat/gamestats).
DEAD_GS_CIDR='69.10.0.0/16'

block_dead_gamespy() {
	if ! command -v iptables >/dev/null 2>&1; then
		echo "iptables not found; skip dead GameSpy block"
		return 0
	fi
	if iptables -C OUTPUT -d "${DEAD_GS_CIDR}" -j REJECT --reject-with icmp-host-unreachable 2>/dev/null; then
		echo "dead GameSpy REJECT already present for ${DEAD_GS_CIDR}"
		return 0
	fi
	iptables -I OUTPUT -d "${DEAD_GS_CIDR}" -j REJECT --reject-with icmp-host-unreachable
	echo "REJECT outbound to ${DEAD_GS_CIDR} (cached dead GameSpy IPs fail fast)"
}

unblock_dead_gamespy() {
	if ! command -v iptables >/dev/null 2>&1; then
		echo "iptables not found; nothing to unblock"
		return 0
	fi
	if iptables -C OUTPUT -d "${DEAD_GS_CIDR}" -j REJECT --reject-with icmp-host-unreachable 2>/dev/null; then
		iptables -D OUTPUT -d "${DEAD_GS_CIDR}" -j REJECT --reject-with icmp-host-unreachable
		echo "removed REJECT for ${DEAD_GS_CIDR}"
	else
		echo "no dead GameSpy REJECT rule present"
	fi
}

case "${1:-}" in
	uninstall)
		need_root "$@"
		uninstall_hosts
		;;
	block-dead)
		need_root "$@"
		block_dead_gamespy
		;;
	unblock-dead)
		need_root "$@"
		unblock_dead_gamespy
		;;
	-h|--help|help)
		usage
		;;
	install)
		# kept for callers that still pass install
		need_root "$@"
		install_hosts "${2:-}"
		;;
	*)
		need_root "$@"
		install_hosts "${1:-}"
		;;
esac
