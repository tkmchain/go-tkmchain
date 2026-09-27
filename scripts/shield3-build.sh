#!/usr/bin/env bash
set -euo pipefail

# Compatibility entry point retained for existing build scripts. The native
# archive now contains both Shield3 and Shield4, so all builds go through the
# stricter Shield4 artifact validation.
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec "$root/scripts/shield4-build.sh" "$@"
