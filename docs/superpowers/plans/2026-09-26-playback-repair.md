# Playback repair implementation plan

**Goal:** Restore audible YouTube and Spotify-link playback in the existing Docker deployment, fix verified correctness defects, and prepare factual free-hosting guidance.

**Architecture:** Keep DiscordGo, disgolink, and Lavalink. Spotify provides metadata and resolves audio through YouTube. Preserve secrets in the existing untracked `.env`.

## Evidence

- September 18 logs show YouTube searches succeed but all playback clients fail with youtube-plugin 1.18.0. Version 1.18.1 fixes OAuth propagation; 1.18.2 fixes format 18 playback.
- September 26 containers stopped during command registration. Bot exit 2, Lavalink exit 143, no OOM or logged Go panic. Do not claim an unproven crash cause.
- Existing baseline `go test ./...` builds successfully but contains no tests.
- Voice readiness is signaled before forwarding updates to Lavalink. Startup voice handlers can dereference an uninitialized client.
- Queue footer format and parser disagree. `/play` omits the same-channel check. Lyrics state exposes mutable pointers. Last-track skip button omits an interaction response.

## Ordered work

- [x] Update YouTube plugin alone; start Docker; observe a real playback attempt in GDGC ISSATSO / Gaming Lobby 1.
- [x] Add regression tests for command authorization, queue pagination, last-track skip responses, lyric-state concurrency and matching. Fix confirmed defects in `internal/commands`, `internal/lyrics`, `internal/utils`, and `internal/embed`.
- [x] Add regression tests for startup events, voice readiness ordering, track metadata without URI, provider failure handling, queue advancement, paused-track replacement, and cancellation. Fix in `internal/bot`, `internal/player`, and `internal/lavalink`.
- [x] Update Lavalink and LavaSrc to verified stable versions; update Go dependencies/toolchain based on compatibility and vulnerability evidence. Keep deployment version pins consistent; legacy cloud workflows are not deployment-tested.
- [x] Harden Docker: exclude secrets from build context, use supported runtime/build images, support ARM64, authenticate health checks, keep Lavalink private, bound logs and memory.
- [x] Run `go test -race ./...`, `go vet ./...`, vulnerability checks, Docker build/config checks, and review the complete diff. Add CI for repeatable verification.
- [ ] Test YouTube search/link and Spotify link in Discord; inspect voice connection, track progress, exceptions, and obtain user confirmation of audible audio. Test pause/resume/skip/queue and restart.
- [x] Verify free hosting offers against official provider documentation; document resource/network constraints and exact Docker deployment steps.
- [ ] Provision only after provider/account choice and access are known.

## Acceptance

Automated checks and container health are necessary but do not prove audible playback. Record observed results separately from pending user confirmation. Do not claim free cloud deployment or production readiness before their respective checks are complete.

## Latest evidence (2026-09-27)

The live YouTube search/URL test passed at 09:50 UTC on Lavalink 4.2.2, including pause, seek, resume, enqueue, skip from paused playback, natural track-end queue advancement, and stop. The test ran in the explicitly resolved Gaming Lobby 1 voice channel. Audible and slash-command/button confirmation remain pending.

Spotify failed again at 09:51 UTC with HTTP 403: the API requires an active Premium subscription for the developer app owner. This is an external account prerequisite, not a remaining code update. See the repair report and deployment guide for remaining acceptance steps.

User follow-up on September 27: audible playback confirmed; Spotify explicitly deferred because the app owner has no Premium. Oracle signup subsequently failed and the user requested other free options, plus meaningful commits and a push before deployment. The deployment guide now separates existing-hardware hosting from eligibility- and credit-limited cloud offers. Publish on a repair branch because pushing `main` triggers the existing Fly deployment workflow. Host selection and remote playback verification remain pending.
