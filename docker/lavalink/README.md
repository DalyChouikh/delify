# Local YouTube CDN fallback

The local Compose stack builds `delify-lavalink:local` using Lavalink 4.2.2 and the existing pinned youtube-source plugin. The upstream image digest and downloaded plugin/source checksums are fixed. Only `YoutubeAudioTrack` and its nested classes are recompiled and replaced in the plugin jar.

The patch checks the selected signed media URL with a one-byte HTTP range request. On an HTTP or transport failure, it checks the alternate hostname advertised by the URL's `mn` parameter once. It preserves the signed query exactly and propagates the error if neither route works. The selected URL then follows the upstream playback path, including its total-length probe. A healthy primary does not contact the alternate.

This addresses the observed October 9 primary CDN connection timeouts. An IP override did not work: the actual stream returned HTTP 400. Changing the hostname to the advertised alternate, following redirects, returned valid MP4 bytes.

Build and start locally:

```sh
docker compose build lavalink
docker compose up -d lavalink bot
```

The build runs five deterministic regression cases against real local HTTP endpoints: HTTP failure (including signature preservation), connection refusal, healthy primary, missing alternate, and bounded failure on both hosts.

To verify the test fails against the unmodified pinned plugin:

```sh
docker build --target regression-before -f docker/lavalink/Dockerfile .
```

That command must fail with `failed primary CDN must select the advertised alternate`; a normal image build must pass all cases. CI builds the patched image and runs these tests.

`docker-compose.prod.yml` still uses the upstream image. This repair currently applies to the local Compose stack. Removing the local `build` configuration and restoring the upstream Lavalink image reference rolls it back.
