# Discord audio diagnosis — October 9, 2026

## Confirmed observations

- The bot connects to Discord, registers commands, and connects to Lavalink 4.2.2.
- September 27 container logs show a YouTube track ending with `reason=finished`; the existing repair report also records audible confirmation on that date.
- October 9 playback attempts fail while Lavalink fetches the audio stream. YouTube search and metadata loading succeed.
- The primary media hosts `rr1---sn-u0opnpxuxa-u0oz.googlevideo.com` and `rr2---sn-u0opnpxuxa-u0oz.googlevideo.com` resolve to `41.224.34.8` and `41.224.34.9`. HTTPS connection attempts time out from both the host machine and Lavalink container.
- HTTPS requests to `www.youtube.com` and the configured cipher service return HTTP 200 from Lavalink.
- Rebuilding the bot succeeds. The old and rebuilt binaries have the same SHA-256: `c02af3dfc371bd81d50fcd0b4a6d2df257ccfca4abf7ccff9c355cac4a865bdd`. Both services' environment values match the base Compose configuration.
- A temporary IP override makes the primary hostnames connect to reachable alternate CDN addresses, but the actual signed media request receives HTTP 400 with zero bytes. This override does not repair playback.
- Changing only the hostname in the actual signed media URL to the alternate advertised by its `mn` parameter produces HTTP 302, followed by HTTP 200 from `rr4---sn-4g5ednd7.googlevideo.com`. A bounded request downloads 65,536 bytes with `Content-Type: video/mp4`; the sample begins with the MP4 `ftyp` header.
- An isolated Lavalink session, with no Discord voice connection, reproduces `TVHTML5: Not success status code: 400` when the IP override is active. All other playback clients fail with login required, no supported audio streams, or video unavailable.
- The same isolated session loads the alternate media URL through Lavalink's HTTP source and produces `TrackStartEvent` without a source exception during an 18-second observation window. Because it has no voice connection, this test does not establish audible playback or voice position progression.
- One Discord interaction acknowledgment also times out at 19:57:20 UTC. A later attempt at 19:57:47 reaches Lavalink and fails fetching media; that later failure is independent of the acknowledgment timeout.

## Source inspection

The exact configured youtube-source commit is `2be8e542d3f6f178e048dca565892684c2e40177`. Its source archive was fetched from GitHub and inspected locally. `YoutubeAudioTrack.FormatWithUrl.getFallback()` constructs an alternate host URL from `mn`, but no playback code calls this method. Playback currently switches InnerTube clients after the primary host fails instead of trying the supplied alternate CDN hostname.

## Implemented local repair

The local Compose stack now builds `delify-lavalink:local` with a narrowly patched version of the same pinned plugin. A one-byte HTTP range request validates the selected media URL before playback. On HTTP or transport failure, the plugin checks the supplied alternate CDN hostname once before changing InnerTube clients. It keeps signed query parameters intact and passes the selected URL into the existing playback and content-length handling. No CDN IP addresses are hard-coded.

The upstream image digest and downloaded source/plugin checksums are pinned. Only `YoutubeAudioTrack` and its nested classes are replaced in the plugin jar. The runtime image carries `dev.delify.youtube-cdn-fallback=1`. See [build and test instructions](../docker/lavalink/README.md).

## TDD and deployment verification

- A regression test against the unmodified plugin failed with `failed primary CDN must select the advertised alternate`.
- After patching, all five regression cases pass: HTTP failure and preserved signature, connection refusal, healthy primary, absent alternate, and bounded failure on both hosts. These exercise the plugin's real URL-selection method with local HTTP servers.
- The patched image builds successfully; its build runs the regression suite. CI now also builds and tests this image.
- `docker compose config --quiet` and `git diff --check` pass.
- Both local containers were recreated. The bot connected to Discord and Lavalink and logged `bot is ready!`.
- At 22:11:46 UTC, the patched server logged a failure on `rr1---sn-u0opnpxuxa-u0oz.googlevideo.com` followed by a retry on `rr1---sn-2ohpa5-5h.googlevideo.com`.
- An isolated test of the original YouTube URL for video `Z2EsL8zdibM` passed: `TrackStartEvent`, no source exception during 18 seconds, and `track_still_present=true`. The test exited with code 0. It does not join Discord voice and does not establish audible playback or voice position progression.

The earlier temporary IP overrides and diagnostic logging were removed. After deployment, the user confirmed audible Discord playback: “yeah there is sound, it played.” Local playback acceptance is complete. `docker-compose.prod.yml` continues to use the upstream Lavalink image; this patch currently applies to the local stack.

## CI security update

The first CI run for PR #3 passed race tests and static analysis, then failed its vulnerability check with nine reachable Go standard-library vulnerabilities in Go 1.26.8. Every finding listed Go 1.26.9 as the fixed version. The module directive and bot Docker builder now use Go 1.26.9; the vulnerability check remains enabled.

Fresh local verification with Go 1.26.9 passed race tests, static analysis, and the bot Docker build. `govulncheck` exited successfully and reported zero vulnerabilities affecting the code. It also reported one module-level advisory in code the bot does not call.

## Diagnostic artifacts

Diagnostic programs and temporary Compose overrides are under `/tmp`. They use an independent Lavalink session and never join Discord voice. Signed media URLs and credential values are excluded from this report and tool output.
