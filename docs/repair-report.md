# Playback repair results

Last verified: 2026-09-27. This report covers the repaired Docker deployment and local playback; cloud deployment remains pending. Existing `.env` credentials were preserved and excluded from publication.

## Confirmed failures

- YouTube searches returned tracks, but the original youtube-source 1.18.0 clients failed to stream them. Tagged 1.18.2 also reproduced `TVHTML5: The page needs to be reloaded`. Playback passed after pinning upstream commit `2be8e542d3f6f178e048dca565892684c2e40177`, containing the [TV client fix](https://github.com/lavalink-devs/youtube-source/pull/233) and [cipher connection fix](https://github.com/lavalink-devs/youtube-source/pull/245). This is a pinned snapshot, not a stable plugin release.
- Spotify returned HTTP 403 at 09:51 UTC on September 27, including the exact message: `Active premium subscription required for the owner of the app.` This remained after updating LavaSrc. [Spotify documents this development-mode requirement](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide). The API response also warns that subscription changes can take hours to become effective.
- The earlier container exits alone did not establish a crash cause: exit codes were 2/143, without a logged Go panic or OOM evidence. Separately, a startup voice-event nil dereference was reproduced in a unit test and fixed.

## Implemented repairs

- Stable per-play IDs for queue event matching, including repeated songs and Lavalink re-encoding tracks with their current position.
- Synchronous player snapshots, per-guild control serialization, cancellation/draining during shutdown, and discarded loads after voice disconnect.
- Correct voice readiness ordering, search fallback on provider exceptions/empty results, paused-track replacement, nil metadata handling, and inactivity timer cleanup.
- Same-channel authorization, guild-only interaction guards, prompt acknowledgments before slow operations, correct button completion and queue pagination.
- Bounded lyrics caches/state, snapshot-safe pagination, stricter matching, Unicode-safe truncation, and validated seek time parsing.
- Authenticated Lavalink health checks, cancellation-aware startup, log credential redaction, and bounded request contexts.

## Updated runtime

| Component | Version |
| --- | --- |
| Go | 1.26.8 |
| DiscordGo | 0.29.0 |
| Lavalink | 4.2.2 |
| LavaSrc | 4.8.3 |
| youtube-source | Pinned commit above |
| Runtime base | Alpine 3.24 |

The Dockerfile supports AMD64/ARM64; the local build and live test were AMD64. Lavalink port 2333 is private to Compose. The bot runs non-root with a read-only filesystem, dropped capabilities, bounded memory/logs, and secrets excluded from the build context. Legacy cloud version pins were updated, but those deployments were not exercised. Fly deployment now requires CI first.

## Verification

Passed on the repaired code:

- `go test -race -count=1 ./...`
- `go vet ./...`, formatting checks, and `git diff --check`
- `govulncheck@v1.8.0`: zero affected code paths and zero vulnerable imported packages. One module-only advisory concerns unused `golang.org/x/crypto/openpgp` (GO-2026-5932; no fixed version); the bot does not import it.
- Both Compose configuration checks and the Docker bot build.
- Live playback test in **GDGC ISSATSO / 🎮 Gaming Lobby 1**, September 27, 09:50 UTC: YouTube text search, voice connected, more than ten seconds of position progression, pause, seek, resume, URL enqueue, skip while paused, replacement playback, natural track-end queue advancement, and stop. The test exited successfully in 30.13 seconds.

The live test uses the real Discord voice gateway and Lavalink API, not a simulated audio service. It does not listen to the channel or synthesize a user's slash-command interaction. Unit tests exercise interaction handling with an HTTP substitute.

## Remaining acceptance requirements

The user confirmed audible playback on September 27 and explicitly deferred Spotify because the app owner does not have Premium. Spotify is no longer a blocker for the current deployment scope; its code and credentials have not been removed.

1. Exercise the remaining Discord slash commands/buttons; real audio API control tests have passed.
2. If Spotify is resumed later, satisfy its account requirement and repeat live track/playlist checks.
3. Choose existing hardware or an eligible cloud offer, then deploy and repeat voice tests from that host. Oracle signup was unsuccessful and the user requested alternatives. See [deployment instructions](deployment.md). No free hosting availability or cloud playback guarantee has been made.

Cloud deployment and its playback checks remain incomplete. Local audible playback is confirmed.
