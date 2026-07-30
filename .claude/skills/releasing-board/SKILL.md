---
name: releasing-board
description: Use when cutting, shipping, tagging, or rolling back a board release — publishing cross-platform binaries to GitHub Releases, diagnosing a release run that failed or landed with missing assets, or fixing a release that shipped a stale web UI.
---

# Releasing Board

## Overview

There is no release script and **no version constant anywhere in the Go source** — the tag *is* the
version. Pushing a tag matching `v*` triggers `.github/workflows/release.yml`, which builds the web UI,
cross-compiles four static binaries, and creates the GitHub Release with `--generate-notes`.

`install.sh` downloads `https://github.com/samuelloranger/board/releases/latest/download/board_${os}_${arch}`.
So **the asset names are the contract** — `board_linux_amd64`, `board_linux_arm64`, `board_darwin_amd64`,
`board_darwin_arm64`. Rename one and every install breaks.

## Pre-flight

Everything below must pass on `main` before you tag. These are exactly what CI runs.

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go build ./...
./install_test.sh                                    # dry-run installer smoke test
(cd internal/web/ui && bun install && bun run build) # then check git status
```

**The last one is the trap.** `internal/web/ui/dist/` is committed and embedded via `//go:embed`. If
the build dirties `dist/`, the tree is carrying a stale UI — commit the rebuilt assets to `main` *before*
tagging, or the release binaries ship the old interface with no error anywhere.

Also confirm CI is green on the commit you're about to tag: `gh run list --workflow CI --limit 1`.

## Cutting the release

Nothing is bumped. Tag the commit and push the tag:

```bash
git switch main && git pull
git tag v0.2.1                # next semver after `git tag | sort -V | tail -1`
git push origin v0.2.1
gh run watch $(gh run list --workflow release --limit 1 --json databaseId -q '.[0].databaseId')
```

The `release` workflow (single `build` job, ubuntu-latest, Go 1.26 + Bun) then:

1. builds `internal/web/ui` with Bun (CI's copy of `dist/`, not yours — but the Go embed uses whatever
   is on disk after that step, which is why a stale committed `dist/` only matters for local builds and
   for anyone building from source);
2. cross-compiles with `CGO_ENABLED=0 GOOS/GOARCH` and `-ldflags "-s -w"` into `dist_release/`;
3. publishes idempotently — `gh release create` if the tag has no release, otherwise
   `gh release upload --clobber` onto the existing one.

## Verify

```bash
gh release view v0.2.1                      # expect exactly the 4 board_${os}_${arch} assets
curl -fsSL https://github.com/samuelloranger/board/releases/latest/download/board_linux_amd64 -o /tmp/b \
  && chmod +x /tmp/b && /tmp/b board
```

`--generate-notes` writes the notes from merged PRs; edit the title/body afterwards with
`gh release edit` if you want a human summary (v0.2.0 has one).

## Rolling back

The release is public the moment CI creates it — there is no draft gate here, so a bad release *is*
live for installers. Fix in this order:

| Need | Command |
|---|---|
| Take a bad release out of `latest` | `gh release delete v0.2.1 --cleanup-tag` |
| Keep the tag, hide it from installs | `gh release edit v0.2.1 --prerelease` |
| Re-upload a corrected asset onto an existing release | `gh release upload v0.2.1 dist_release/board_linux_amd64 --clobber` |

`install.sh` resolves `/releases/latest/download/...`, which skips prereleases and deleted releases —
so either of the first two immediately falls installs back to the previous good release. Do that first,
diagnose second. Then fix forward and cut the **next** patch version; re-tagging the same version means
force-pushing a tag users may already have fetched.

## Landmines

- **A tag with no release still looks like a version.** If the workflow fails, the tag exists but
  `latest` is unchanged, so users are fine. Delete the tag before retrying: a re-push of the same tag
  does not re-trigger the workflow.
- **Actions can be billing-blocked on this account** (it has happened). The workflow header documents
  the local fallback verbatim — build the four binaries with the same `CGO_ENABLED=0 GOOS/GOARCH` loop
  and `gh release create vX.Y.Z dist_release/* --title vX.Y.Z --generate-notes`. Build the web UI first.
- **`dist_release/` is gitignored.** Never commit it; it exists only as a build/upload staging dir.
- `plugin/.claude-plugin/plugin.json` carries `"version": "0.1.0"`, which has drifted behind the release
  tags. It is not read by the release pipeline. TODO(sam): decide whether the plugin version should track
  release tags (and get bumped as part of this procedure) or stay independent.
- TODO(sam): binaries are unsigned and unnotarized. macOS Gatekeeper will quarantine a curl-downloaded
  `board_darwin_*` — confirm whether users need `xattr -d com.apple.quarantine` and document it if so.
