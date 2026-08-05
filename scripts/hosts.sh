#!/usr/bin/env bash
# Install or remove Nintendo WFC host aliases in /etc/hosts (needs sudo).
set -euo pipefail

HOSTS_FILE="/etc/hosts"
BEGIN='# <!-- mkw-dwc -->'
END='# <!-- /mkw-dwc -->'

NAMES=(
	naswii.nintendowifi.net
	nas.nintendowifi.net
	mariokartwii.available.gs.nintendowifi.net
	mariokartwii.master.gs.nintendowifi.net
	mariokartwii.ms19.gs.nintendowifi.net
	mariokartwii.natneg1.gs.nintendowifi.net
	mariokartwii.natneg2.gs.nintendowifi.net
	mariokartwii.natneg3.gs.nintendowifi.net
	gpcm.gs.nintendowifi.net
	gpsp.gs.nintendowifi.net
)

usage() {
	echo "Usage: $0 [ip] | uninstall"
	echo "  [ip]          add aliases (default: 127.0.0.1)"
	echo "  uninstall     remove managed aliases"
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

case "${1:-}" in
	uninstall)
		need_root "$@"
		uninstall_hosts
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
