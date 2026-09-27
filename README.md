# Delify

A Go Discord music bot using Lavalink for voice playback and LavaSrc for Spotify metadata. Spotify links are matched to audio on YouTube; the bot does not stream audio from Spotify.

## Run with Docker

1. For a **new installation only**, copy `.env.example` to `.env`. Preserve an existing secret file.
2. Set `DISCORD_TOKEN` and a strong `LAVALINK_PASSWORD`. Set Spotify app credentials if using Spotify links.
3. Invite the bot with `bot` and `applications.commands` scopes and View Channel, Send Messages, Embed Links, Connect, and Speak permissions. Slash commands do not need Message Content intent.
4. Start the services:

```sh
docker compose up -d --build --wait
docker compose ps
docker compose logs --tail=100 bot
```

Join a voice channel and use `/play` with a song name or link. Run only one bot instance per Discord token. Containers restart after a host reboot unless manually stopped. Queues are held in memory and are lost on restart.

Lavalink is reachable only inside the Docker network; port 2333 is intentionally not published. Outbound HTTPS/WebSocket and Discord voice UDP must be permitted. Neither service needs public inbound traffic.

## Commands

| Command | Action |
| --- | --- |
| `/play query` | Play a song, URL, or playlist; enqueue while already playing |
| `/pause`, `/resume` | Pause/resume playback |
| `/skip` | Skip the current track |
| `/stop` | Clear the queue and leave voice |
| `/nowplaying` | Track information and playback buttons |
| `/queue page` | Paginated queue, ten tracks per page |
| `/clear` | Clear queued tracks while keeping the current track |
| `/seek time` | Seek with seconds, `m:ss`, or `h:mm:ss` |
| `/lyrics query` | Optional Genius lyrics lookup |

Playback controls require the user's voice channel to match the bot's. Text search tries YouTube first, then SoundCloud when searching fails or returns no results. Explicit `ytsearch:`, `ytmsearch:`, `scsearch:`, and `spsearch:` prefixes are supported. A failure *during playback* is logged and advances the queue; search fallback does not replace it with a different recording.

## Configuration

| Variable | Purpose |
| --- | --- |
| `DISCORD_TOKEN` | Required bot token |
| `DISCORD_GUILD_IDS` | Optional comma-separated guild IDs; empty registers globally |
| `LAVALINK_PASSWORD` | Shared audio-server password |
| `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET` | Spotify developer app credentials |
| `YOUTUBE_OAUTH_ENABLED` | Enable YouTube authentication; default false |
| `YOUTUBE_OAUTH_REFRESH_TOKEN` | Existing YouTube OAuth refresh token |
| `YT_CIPHER_URL` | Signature-decipher service; defaults to https://cipher.kikkia.dev/ |
| `YT_CIPHER_USER_AGENT` | Identifier sent to the decipher service |
| `RAPIDAPI_KEY`, `RAPIDAPI_HOST` | Optional Genius lyrics API credentials |
| `INACTIVITY_TIMEOUT` | Seconds before leaving idle voice; default 30 |
| `LOG_LEVEL` | debug, info, warn, or error; default info |
| `DEVELOPER_USER_ID` | Optional footer avatar |

Compose supplies `LAVALINK_HOST`, `LAVALINK_PORT`, and `LAVALINK_SECURE`. Running the Go binary directly requires exporting these and credentials; it does not automatically read `.env`.

### YouTube compatibility

The configuration pins youtube-source commit `2be8e542d3f6f178e048dca565892684c2e40177` from the upstream snapshot repository. Tagged release 1.18.2 still reproduced `TVHTML5: The page needs to be reloaded`. The pinned build contains the [TV client fix](https://github.com/lavalink-devs/youtube-source/pull/233) and [cipher connection fix](https://github.com/lavalink-devs/youtube-source/pull/245). It is a pinned snapshot, not a stable release or a floating latest tag.

The configured TV client requires a valid OAuth refresh token. Other clients have different requirements and may stop working when YouTube changes. Follow the [upstream OAuth instructions](https://github.com/lavalink-devs/youtube-source#using-oauth-tokens). Keep refresh tokens out of source control and avoid sharing unredacted Lavalink logs; OAuth setup can print credentials. The public decipher service is an external dependency, not an availability guarantee. Upstream documents self-hosting alternatives.

### Spotify requirements

Spotify development-mode apps require their owner to have Premium. An HTTP 403 saying an active subscription is required is an account condition; changing bot code or rotating the same credentials does not fix it. See [Spotify's migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide).

LavaSrc 4.8.3 includes Spotify playlist endpoint updates. Credential validity and playlist access are still checked by Spotify. Music can be searched by artist/title on YouTube independently of Spotify API access.

## Verification

Requires Go 1.26.8 or newer:

```sh
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
docker compose config --quiet
docker compose -f docker-compose.prod.yml config --quiet
docker compose build bot
```

CI runs these checks on pushes and pull requests. Unit tests do not contact Discord or music providers. The opt-in live test joins an explicitly named voice channel and checks connection and ten seconds of track progression. Use your own test channel with a listener present.

```sh
CGO_ENABLED=0 go test -c -o /tmp/delify-live.test ./internal/bot
docker compose stop bot
docker compose up -d --wait lavalink
docker compose run --rm --no-deps \
  --entrypoint /tmp/delify-live.test \
  -v /tmp/delify-live.test:/tmp/delify-live.test:ro \
  -e DELIFY_TEST_GUILD='Your exact server name' \
  -e DELIFY_TEST_CHANNEL='Your voice channel name' \
  -e DELIFY_TEST_QUERY='Your song or URL' \
  bot -test.run '^TestLivePlayback$' -test.v -test.timeout 110s
docker compose up -d bot
```

Use an exact voice-channel name or its numeric ID; ambiguous names are rejected. Add `-e DELIFY_TEST_CONTROLS=true` to the test container to also verify pause, seek, resume, URL enqueue, skip from a paused track, natural queue advancement, and stop through the real audio API.

Restart the normal bot even if the test fails. Test success confirms connection/progress and API controls; ask the listener to verify audible audio and exercise the Discord slash commands/buttons.

## Hosting and operation

See [free hosting and deployment](docs/deployment.md). The Dockerfile supports AMD64 and ARM64. Compose bounds memory and logs, and runs the bot without root, write access or Linux capabilities. Secrets are excluded from build context and runtime images.

`docker-compose.prod.yml` uses a prebuilt image selected by `BOT_IMAGE`. Existing Fly, GCP and Azure files are deployment examples, not evidence that those providers will host the bot free. Local checks do not certify cloud deployments.

The repair branch is `fix/reliable-music-playback`. Pushing it runs CI without invoking the Fly deployment workflow. Pushing or merging to `main` **does** trigger that workflow; approve the deployment destination before merging.

```sh
docker compose logs --since=10m bot lavalink
docker stats --no-stream delify-bot delify-lavalink
docker compose restart bot
docker compose stop
```

Docker health means the audio API responds with the configured password. It does not guarantee that YouTube/Spotify playback is available. Never share `.env`, tokens or unredacted authentication logs.
