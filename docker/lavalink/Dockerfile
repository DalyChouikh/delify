FROM ghcr.io/lavalink-devs/lavalink:4.2.2@sha256:87ae53e60dc147c9dddb28e126ce503a26bc8d1477ed8d99543614677882afff AS upstream

FROM eclipse-temurin:21-jdk-alpine AS dependencies
RUN apk add --no-cache curl patch
WORKDIR /deps
COPY --from=upstream /opt/Lavalink/Lavalink.jar ./Lavalink.jar
ARG YOUTUBE_VERSION=2be8e542d3f6f178e048dca565892684c2e40177
RUN curl -fsSL --connect-timeout 10 --max-time 60 \
    "https://maven.lavalink.dev/snapshots/dev/lavalink/youtube/youtube-plugin/${YOUTUBE_VERSION}/youtube-plugin-${YOUTUBE_VERSION}.jar" \
    -o youtube.jar \
    && echo 'b062effdda6abeaa46913dfd1b5d5600120b65f239ce88f30948c7213a3ca1c0  youtube.jar' | sha256sum -c - \
    && jar xf Lavalink.jar BOOT-INF/lib
COPY docker/lavalink/CdnFallbackTest.java /tests/CdnFallbackTest.java
RUN javac --release 11 -cp '/deps/BOOT-INF/lib/*:/deps/youtube.jar' \
    -d /tests /tests/CdnFallbackTest.java

# Intentionally fails against the unmodified plugin; used to verify the regression test.
FROM dependencies AS regression-before
RUN java -cp '/tests:/deps/BOOT-INF/lib/*:/deps/youtube.jar' \
    dev.lavalink.youtube.track.CdnFallbackTest

FROM dependencies AS patched
RUN curl -fsSL --connect-timeout 10 --max-time 60 \
    "https://raw.githubusercontent.com/lavalink-devs/youtube-source/${YOUTUBE_VERSION}/common/src/main/java/dev/lavalink/youtube/track/YoutubeAudioTrack.java" \
    -o YoutubeAudioTrack.java \
    && echo '06069af5415293a6060c3575305078f7e46e4710bc482a90507bd5cf19785086  YoutubeAudioTrack.java' | sha256sum -c -
COPY docker/lavalink/youtube-cdn-fallback.patch /deps/youtube-cdn-fallback.patch
RUN patch --fuzz=0 YoutubeAudioTrack.java < youtube-cdn-fallback.patch \
    && javac --release 11 -cp '/deps/BOOT-INF/lib/*:/deps/youtube.jar' \
       -d /patched YoutubeAudioTrack.java \
    && jar uf youtube.jar -C /patched dev/lavalink/youtube/track
RUN java -cp '/tests:/deps/BOOT-INF/lib/*:/deps/youtube.jar' \
    dev.lavalink.youtube.track.CdnFallbackTest

FROM upstream AS runtime
ARG YOUTUBE_VERSION=2be8e542d3f6f178e048dca565892684c2e40177
LABEL dev.delify.youtube-cdn-fallback="1"
COPY --from=patched --chown=lavalink:lavalink /deps/youtube.jar \
    /opt/Lavalink/plugins/youtube-plugin-${YOUTUBE_VERSION}.jar
