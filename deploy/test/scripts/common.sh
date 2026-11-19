# Shared helpers for the container-side test scripts. Sourced, not executed.
set -euo pipefail

step() { printf '\n==> %s\n' "$*"; }

# copy_tree SOURCE TARGET copies a read-only checkout into a writable directory,
# leaving out local secrets, dependencies and build output.
copy_tree() {
	mkdir -p "$2"
	tar -C "$1" \
		--exclude=./.git --exclude=./bin --exclude=./node_modules --exclude=./frontend/node_modules \
		--exclude=./dist --exclude=./frontend/dist --exclude=./.env --exclude=./backend/.env --exclude=./frontend/.env \
		-cf - . | tar -C "$2" -xf -
}

# give_back DIR hands files written as root back to the host user (Docker only;
# rootless Podman already maps container root to that user).
give_back() {
	if [ -n "${HOST_UID:-}" ] && [ -n "${HOST_GID:-}" ]; then
		chown -R "$HOST_UID:$HOST_GID" "$1" || true
	fi
}
