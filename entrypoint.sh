#!/bin/sh
set -eu

# GitHub creates these command-channel files as the host runner user. A Docker
# Action starts as root, grants the dedicated analyzer group write access only
# inside GitHub's command directory, then drops privileges before parsing any
# repository-controlled content. The host owner remains unchanged.
for channel in "${GITHUB_STEP_SUMMARY:-}" "${GITHUB_OUTPUT:-}"; do
  case "$channel" in
    /github/file_commands/*)
      resolved="$(readlink -f "$channel")"
      case "$resolved" in
        /github/file_commands/*)
          if [ -f "$resolved" ]; then
            channel_dir="$(dirname "$resolved")"
            chgrp depprism "$channel_dir"
            chmod g+x "$channel_dir"
            chgrp depprism "$resolved"
            chmod g+rw "$resolved"
          fi
          ;;
      esac
      ;;
  esac
done

exec su-exec depprism:depprism /usr/local/bin/depprism "$@"
