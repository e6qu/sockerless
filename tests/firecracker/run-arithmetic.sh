#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

repo_root="$(git rev-parse --show-toplevel)"
workdir="${FIRECRACKER_WORKDIR:-$repo_root/.firecracker-ci}"
version="${FIRECRACKER_VERSION:-v1.15.1}"
arch="$(uname -m)"
tap_dev="${FIRECRACKER_TAP_DEV:-fc-ci-tap0}"
tap_ip="172.16.0.1"
guest_ip="172.16.0.2"
guest_mac="06:00:AC:10:00:02"
api_socket="$workdir/firecracker.socket"
fc_pid=""
nat_iface=""

cleanup() {
  status=$?
  if [ -n "$fc_pid" ] && kill -0 "$fc_pid" >/dev/null 2>&1; then
    sudo kill "$fc_pid" >/dev/null 2>&1 || true
    wait "$fc_pid" >/dev/null 2>&1 || true
  fi
  if [ -n "$nat_iface" ]; then
    sudo iptables -t nat -D POSTROUTING -o "$nat_iface" -j MASQUERADE >/dev/null 2>&1 || true
  fi
  sudo ip link del "$tap_dev" >/dev/null 2>&1 || true
  sudo rm -f "$api_socket" >/dev/null 2>&1 || true
  if [ "$status" -ne 0 ] && [ -d "$workdir" ]; then
    echo "Firecracker workdir retained for inspection: $workdir" >&2
  elif [ -d "$workdir" ]; then
    sudo rm -rf "$workdir"
  fi
}
trap cleanup EXIT

[ "$(uname -s)" = "Linux" ] || fail "Firecracker CI test requires Linux"
[ -e /dev/kvm ] || fail "Firecracker CI test requires /dev/kvm"
if ! { [ -r /dev/kvm ] && [ -w /dev/kvm ]; }; then
  sudo test -r /dev/kvm && sudo test -w /dev/kvm || fail "Firecracker CI test requires read/write access to /dev/kvm"
fi

need_cmd curl
need_cmd firecracker
need_cmd go
need_cmd ip
need_cmd iptables
need_cmd mkfs.ext4
need_cmd ssh
need_cmd ssh-keygen
need_cmd sudo
need_cmd tar
need_cmd unsquashfs

case "$arch" in
  x86_64|aarch64) ;;
  *) fail "Firecracker CI test supports x86_64 and aarch64 Linux runners; got $arch" ;;
esac

rm -rf "$workdir"
mkdir -p "$workdir"

ci_version="${version%.*}"
asset_index="$workdir/firecracker-ci-assets.xml"
curl -fsSLo "$asset_index" "https://s3.amazonaws.com/spec.ccfc.min/?prefix=firecracker-ci/${ci_version}/${arch}/&list-type=2"

kernel_key="$(tr '<' '\n' < "$asset_index" | sed -n 's#^Key>\(.*\)#\1#p' | grep "^firecracker-ci/${ci_version}/${arch}/vmlinux-" | sort -V | tail -1)"
rootfs_key="$(tr '<' '\n' < "$asset_index" | sed -n 's#^Key>\(.*\)#\1#p' | grep "^firecracker-ci/${ci_version}/${arch}/ubuntu-.*[.]squashfs$" | sort -V | tail -1)"

[ -n "$kernel_key" ] || fail "could not find Firecracker CI kernel asset for $ci_version/$arch"
[ -n "$rootfs_key" ] || fail "could not find Firecracker CI Ubuntu rootfs asset for $ci_version/$arch"

kernel="$workdir/$(basename "$kernel_key")"
rootfs_squash="$workdir/$(basename "$rootfs_key")"
rootfs_dir="$workdir/rootfs"
rootfs_ext4="$workdir/rootfs.ext4"

curl -fsSLo "$kernel" "https://s3.amazonaws.com/spec.ccfc.min/${kernel_key}"
curl -fsSLo "$rootfs_squash" "https://s3.amazonaws.com/spec.ccfc.min/${rootfs_key}"

unsquashfs -quiet -d "$rootfs_dir" "$rootfs_squash"

ssh_key="$workdir/id_ed25519"
ssh-keygen -q -t ed25519 -N "" -f "$ssh_key"
mkdir -p "$rootfs_dir/root/.ssh"
cp "$ssh_key.pub" "$rootfs_dir/root/.ssh/authorized_keys"
chmod 700 "$rootfs_dir/root/.ssh"
chmod 600 "$rootfs_dir/root/.ssh/authorized_keys"

goroot="$(go env GOROOT)"
[ -d "$goroot" ] || fail "go env GOROOT did not resolve to a directory"
mkdir -p "$rootfs_dir/usr/local"
cp -a "$goroot" "$rootfs_dir/usr/local/go"

mkdir -p "$rootfs_dir/root/sockerless/simulators/testdata"
cp -R "$repo_root/simulators/testdata/eval-arithmetic" "$rootfs_dir/root/sockerless/simulators/testdata/eval-arithmetic"

cat > "$rootfs_dir/root/run-firecracker-arithmetic.sh" <<'GUESTSCRIPT'
#!/bin/sh
set -eu
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export HOME=/root
export GOCACHE=/root/.cache/go-build
cd /root/sockerless/simulators/testdata/eval-arithmetic
go version
go test -count=1 ./...
go build -v -o /root/eval-arithmetic .
check() {
  expr="$1"
  want="$2"
  got="$(/root/eval-arithmetic "$expr")"
  if [ "$got" != "$want" ]; then
    echo "arithmetic mismatch for $expr: expected $want got $got" >&2
    exit 1
  fi
}
check '3 + 4 * 2' '11'
check '(10 - 3) * 2' '14'
check '100 / 5 + 1' '21'
check '2 * (3 + 4) - 1' '13'
check '1.5 + 2.5 * 2' '6.5'
echo FIRECRACKER_ARITHMETIC_OK
GUESTSCRIPT
chmod 755 "$rootfs_dir/root/run-firecracker-arithmetic.sh"

sudo chown -R root:root "$rootfs_dir"
truncate -s 3G "$rootfs_ext4"
sudo mkfs.ext4 -q -d "$rootfs_dir" -F "$rootfs_ext4"

sudo rm -f "$api_socket"
sudo ip link del "$tap_dev" >/dev/null 2>&1 || true
sudo ip tuntap add dev "$tap_dev" mode tap
sudo ip addr add "${tap_ip}/30" dev "$tap_dev"
sudo ip link set dev "$tap_dev" up
sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null

nat_iface="$(ip route show default | awk 'NR == 1 { for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
[ -n "$nat_iface" ] || fail "could not determine default network interface for guest NAT"
sudo iptables -t nat -A POSTROUTING -o "$nat_iface" -j MASQUERADE

sudo firecracker --api-sock "$api_socket" --enable-pci > "$workdir/firecracker-console.log" 2>&1 &
fc_pid=$!

deadline=$((SECONDS + 10))
while [ ! -S "$api_socket" ]; do
  kill -0 "$fc_pid" >/dev/null 2>&1 || fail "Firecracker exited before API socket was created"
  [ "$SECONDS" -lt "$deadline" ] || fail "timed out waiting for Firecracker API socket"
  sleep 0.1
done

fc_put() {
  path="$1"
  payload="$2"
  sudo curl -fsS -X PUT --unix-socket "$api_socket" --data "$payload" "http://localhost${path}" >/dev/null
}

fc_put /logger "{
  \"log_path\": \"$workdir/firecracker.log\",
  \"level\": \"Info\",
  \"show_level\": true,
  \"show_log_origin\": true
}"

boot_args="console=ttyS0 reboot=k panic=1"
if [ "$arch" = "aarch64" ]; then
  boot_args="keep_bootcon $boot_args"
fi

fc_put /boot-source "{
  \"kernel_image_path\": \"$kernel\",
  \"boot_args\": \"$boot_args\"
}"

fc_put /drives/rootfs "{
  \"drive_id\": \"rootfs\",
  \"path_on_host\": \"$rootfs_ext4\",
  \"is_root_device\": true,
  \"is_read_only\": false
}"

fc_put /network-interfaces/net1 "{
  \"iface_id\": \"net1\",
  \"guest_mac\": \"$guest_mac\",
  \"host_dev_name\": \"$tap_dev\"
}"

fc_put /machine-config '{
  "vcpu_count": 2,
  "mem_size_mib": 1024
}'

fc_put /actions '{
  "action_type": "InstanceStart"
}'

ssh_opts=(
  -i "$ssh_key"
  -o BatchMode=yes
  -o ConnectTimeout=2
  -o LogLevel=ERROR
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
)

deadline=$((SECONDS + 120))
until ssh "${ssh_opts[@]}" "root@$guest_ip" true >/dev/null 2>&1; do
  kill -0 "$fc_pid" >/dev/null 2>&1 || fail "Firecracker exited before guest SSH became reachable"
  [ "$SECONDS" -lt "$deadline" ] || fail "timed out waiting for guest SSH at $guest_ip"
  sleep 1
done

ssh "${ssh_opts[@]}" "root@$guest_ip" "ip route replace default via $tap_ip dev eth0 && echo nameserver 1.1.1.1 >/etc/resolv.conf"
ssh "${ssh_opts[@]}" "root@$guest_ip" /root/run-firecracker-arithmetic.sh | tee "$workdir/guest-arithmetic.log"
grep -q FIRECRACKER_ARITHMETIC_OK "$workdir/guest-arithmetic.log" || fail "guest arithmetic smoke did not print success marker"

ssh "${ssh_opts[@]}" "root@$guest_ip" reboot >/dev/null 2>&1 || true
deadline=$((SECONDS + 20))
while kill -0 "$fc_pid" >/dev/null 2>&1; do
  [ "$SECONDS" -lt "$deadline" ] || fail "Firecracker did not exit after guest reboot"
  sleep 1
done
