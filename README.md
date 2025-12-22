# Delify - Discord Music Bot

A production-grade Discord music bot written in Go, powered by Lavalink v4 with Spotify support via LavaSrc.

## 🎵 Features

- **Slash Commands**: Modern Discord slash command interface
  - `/play [query/link]` - Play a song or add to queue (supports YouTube, Spotify, and more)
  - `/skip` - Skip the current track
  - `/stop` - Stop playback and clear the queue

- **Spotify Integration**: Play Spotify tracks by mirroring them via YouTube
- **Queue System**: Automatic queue management with track progression
- **Production Ready**: Clean architecture, proper error handling, graceful shutdown

## 🏗️ Architecture

```
delify/
├── cmd/
│   └── bot/
│       └── main.go           # Application entry point
├── internal/
│   ├── bot/
│   │   └── bot.go            # Bot initialization and lifecycle
│   ├── commands/
│   │   └── handler.go        # Slash command handlers
│   ├── config/
│   │   ├── config.go         # Configuration from env vars
│   │   └── errors.go         # Configuration errors
│   ├── lavalink/
│   │   └── client.go         # Lavalink connection with retry
│   └── player/
│       ├── manager.go        # Music player management
│       ├── queue.go          # Track queue implementation
│       └── types.go          # Shared types
├── docker-compose.yml        # Docker Compose configuration
├── Dockerfile                # Multi-stage Docker build
├── go.mod                    # Go module definition
└── .env.example              # Example environment variables
```

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose
- A Discord Bot Token ([Discord Developer Portal](https://discord.com/developers/applications))
- Spotify API Credentials ([Spotify Developer Dashboard](https://developer.spotify.com/dashboard))

### Setup

1. **Clone and configure:**
   ```bash
   cd delify
   cp .env.example .env
   ```

2. **Edit `.env` with your credentials:**
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   DISCORD_GUILD_ID=your_server_id   # Optional, for faster command registration
   SPOTIFY_CLIENT_ID=your_spotify_client_id
   SPOTIFY_CLIENT_SECRET=your_spotify_client_secret
   ```

3. **Start the bot:**
   ```bash
   docker-compose up --build
   ```

That's it! The bot will automatically:
- Start Lavalink with the LavaSrc plugin
- Wait for Lavalink to be ready
- Connect to Discord and register slash commands

## 🔧 Configuration

All configuration is done via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DISCORD_TOKEN` | Discord bot token | *required* |
| `DISCORD_GUILD_ID` | Guild ID for command registration | *global* |
| `LAVALINK_PASSWORD` | Lavalink server password | `youshallnotpass` |
| `SPOTIFY_CLIENT_ID` | Spotify API client ID | *required for Spotify* |
| `SPOTIFY_CLIENT_SECRET` | Spotify API client secret | *required for Spotify* |
| `YT_CIPHER_URL` | Remote cipher server URL (fallback) | `https://cipher.kikkia.dev/` |
| `YT_CIPHER_USER_AGENT` | Identifier for cipher server logs | `delify-bot` |
| `YOUTUBE_OAUTH_ENABLED` | Enable YouTube OAuth (optional) | `false` |
| `YOUTUBE_OAUTH_REFRESH_TOKEN` | YouTube OAuth refresh token (store securely) | *none* |

### Choosing between OAuth and Remote Cipher

- OAuth: authenticates requests as a signed-in YouTube user and often bypasses signature/cipher issues. Recommended for private bots; use a burner account to avoid risking your personal account.
- Remote cipher server: an external service (public: `https://cipher.kikkia.dev/` or self-hosted) that performs signature decryption. Good if you prefer not to use OAuth.
- You can enable OAuth and still keep the remote cipher configured as a fallback; Lavalink will use clients that support OAuth first.

### Quick steps for OAuth (optional)

1. Start the stack: `docker-compose up lavalink`
2. Check Lavalink logs for a device code and follow the URL it prints (e.g., `https://www.google.com/device`).
3. Authorize with a burner Google account.
4. Copy the refresh token from the logs and add it to your `.env` as `YOUTUBE_OAUTH_REFRESH_TOKEN`.
5. Restart the stack.

### Quick steps for self-hosted `yt-cipher` (optional)

1. Clone and run the service: `git clone https://github.com/kikkia/yt-cipher && cd yt-cipher && docker compose up -d`
2. In `docker-compose.yml`, set `PLUGINS_YOUTUBE_REMOTECIPHER_URL` to `http://yt-cipher:8001/` and (optionally) set `API_TOKEN`.
3. Restart Lavalink.

### Lavalink Plugin Configuration

The `docker-compose.yml` configures Lavalink entirely via environment variables, avoiding the need for a mounted `application.yml`:

```yaml
# Plugin installation at runtime
LAVALINK_PLUGINS_0_DEPENDENCY: "com.github.topi314.lavasrc:lavasrc-plugin:4.3.0"
LAVALINK_PLUGINS_0_SNAPSHOT: false

# LavaSrc configuration
PLUGINS_LAVASRC_PROVIDERS_0: "ytsearch:\"%ISRC%\""
PLUGINS_LAVASRC_PROVIDERS_1: "ytsearch:%QUERY%"
PLUGINS_LAVASRC_SOURCES_SPOTIFY: true
```

## 🌐 Azure Deployment

### Azure Container Apps

1. Create an Azure Container Registry
2. Build and push the image:
   ```bash
   az acr build --registry <your-registry> --image delify:latest .
   ```
3. Deploy as a Container Apps environment with both services

### Azure VM

1. Install Docker on the VM
2. Clone the repository
3. Configure `.env`
4. Run `docker-compose up -d`

## 📝 Development

### Local Development (without Docker)

1. Start Lavalink separately:
   ```bash
   docker run -d -p 2333:2333 \
     -e LAVALINK_SERVER_PASSWORD=youshallnotpass \
     ghcr.io/lavalink-devs/lavalink:4
   ```

2. Run the bot:
   ```bash
   export DISCORD_TOKEN=your_token
   export LAVALINK_HOST=localhost
   go run ./cmd/bot
   ```

### Building

```bash
go build -o delify ./cmd/bot
```

## 📄 License

MIT License - Use it however you like for your private bot!
