# Delify - Discord Music Bot

A production-grade Discord music bot written in Go, powered by Lavalink v4 with Spotify support via LavaSrc.

## 🎵 Features

- **Rich Embeds**: Beautiful now-playing cards with artwork, progress info, and queue status
- **Interactive Buttons**: Control playback without typing commands
  - ▶️ Pause / Resume
  - ⏭️ Skip current track
  - ⏹️ Stop playback
  - 🎤 Lyrics (with pagination)
  - ⏪/⏩ Seek (±5s, ±10s, ±30s)

- **Slash Commands**: Modern Discord slash command interface
  - `/play [query/link]` - Play a song or add to queue (supports YouTube, Spotify, and more)
  - `/skip` - Skip the current track
  - `/stop` - Stop playback and clear the queue
  - `/pause` - Pause the current track
  - `/resume` - Resume playback
  - `/nowplaying` - Show the currently playing track with controls
  - `/queue [page]` - View the queue (paginated, 10 tracks per page)
  - `/clear` - Clear the queue (keeps current song playing)
  - `/seek [time]` - Seek to a position (e.g., `1:30`, `90`, `1:30:00`)
  - `/lyrics [query]` - Show lyrics for current track or search (with pagination)

- **Lyrics Support**: Fetch and display song lyrics via Genius API
- **Auto-Leave**: Bot automatically leaves voice channel after inactivity (configurable)
- **Spotify Integration**: Play Spotify tracks by mirroring them via YouTube
- **Queue System**: Automatic queue management with track progression
- **User-Friendly Errors**: Ephemeral error messages that don't clutter the channel
- **Lyrics Caching**: Lyrics are cached to reduce API calls
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
│   ├── components/
│   │   └── buttons.go        # Discord button components
│   ├── config/
│   │   ├── config.go         # Configuration from env vars
│   │   └── errors.go         # Configuration errors
│   ├── embed/
│   │   ├── builder.go        # Fluent embed builder
│   │   ├── colors.go         # Discord embed colors
│   │   └── templates.go      # Pre-built embed templates
│   ├── errors/
│   │   └── messages.go       # User-friendly error messages
│   ├── lavalink/
│   │   └── client.go         # Lavalink connection with retry
│   ├── lyrics/
│   │   └── client.go         # Genius lyrics API client with caching
│   ├── player/
│   │   ├── manager.go        # Music player management
│   │   ├── queue.go          # Track queue implementation
│   │   └── types.go          # Shared types
│   └── utils/
│       └── format.go         # Time formatting utilities
├── docker-compose.yml        # Docker Compose configuration
├── Dockerfile                # Multi-stage Docker build
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
├── docker-compose.prod.yml   # Production compose for GCE (pre-built images)
├── .github/
│   └── workflows/
│       ├── deploy-fly.yml    # Fly.io CI/CD pipeline (active)
│       ├── deploy-azure.yml  # Azure CI/CD pipeline (legacy/manual)
│       └── deploy-gcp.yml    # GCP CI/CD pipeline (legacy/manual)
└── .env.example              # Example environment variables
```

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose
- A Discord Bot Token ([Discord Developer Portal](https://discord.com/developers/applications))
- Spotify API Credentials ([Spotify Developer Dashboard](https://developer.spotify.com/dashboard))
- (Optional) RapidAPI Key for lyrics ([Genius Song Lyrics API](https://rapidapi.com/Glavier/api/genius-song-lyrics1))

### Setup

1. **Clone and configure:**
   ```bash
   cd delify
   cp .env.example .env
   ```

2. **Edit `.env` with your credentials:**
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   DISCORD_GUILD_IDS=123456789,987654321   # Optional, comma-separated for faster registration
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
| `DISCORD_GUILD_IDS` | Comma-separated guild IDs for faster registration | *global* |
| `DEVELOPER_USER_ID` | Your Discord user ID (for footer avatar) | *optional* |
| `LAVALINK_PASSWORD` | Lavalink server password | `youshallnotpass` |
| `SPOTIFY_CLIENT_ID` | Spotify API client ID | *required for Spotify* |
| `SPOTIFY_CLIENT_SECRET` | Spotify API client secret | *required for Spotify* |
| `YT_CIPHER_URL` | Remote cipher server URL (fallback) | `https://cipher.kikkia.dev/` |
| `YT_CIPHER_USER_AGENT` | Identifier for cipher server logs | `delify-bot` |
| `YOUTUBE_OAUTH_ENABLED` | Enable YouTube OAuth (optional) | `false` |
| `YOUTUBE_OAUTH_REFRESH_TOKEN` | YouTube OAuth refresh token (store securely) | *none* |
| `RAPIDAPI_KEY` | RapidAPI key for Genius lyrics | *optional* |
| `RAPIDAPI_HOST` | RapidAPI host | `genius-song-lyrics1.p.rapidapi.com` |
| `INACTIVITY_TIMEOUT` | Seconds before bot leaves voice when idle | `30` |

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
LAVALINK_PLUGINS_0_DEPENDENCY: "dev.lavalink.youtube:youtube-plugin:1.18.0"
LAVALINK_PLUGINS_0_SNAPSHOT: false
LAVALINK_PLUGINS_1_DEPENDENCY: "com.github.topi314.lavasrc:lavasrc-plugin:4.8.1"
LAVALINK_PLUGINS_1_SNAPSHOT: false

# LavaSrc configuration
PLUGINS_LAVASRC_PROVIDERS_0: "ytsearch:\"%ISRC%\""
PLUGINS_LAVASRC_PROVIDERS_1: "ytsearch:%QUERY%"
PLUGINS_LAVASRC_SOURCES_SPOTIFY: true
```

### Troubleshooting playback

- If logs show `YouTube is no longer supported in this application or device.`, make sure you are on `youtube-plugin:1.18.0` or newer.
- If logs show `websocket closed ... code=4017 reason="E2EE/DAVE protocol required"`, the target Discord voice server requires DAVE/E2EE. Use a voice channel/server where DAVE is not required.

## ☁️ Fly.io Deployment (Recommended)

Delify now ships with Fly.io app configs and CI deployment workflow:

- `fly.lavalink.toml` for Lavalink (private Flycast TCP service on port `2333`)
- `fly.bot.toml` for the Discord bot
- `.github/workflows/deploy-fly.yml` for automatic deploys on push to `main`

### Why this setup

- Lavalink is private-only over Flycast (`delify-lavalink.flycast`), not publicly exposed.
- Bot and Lavalink are split into separate apps for independent deploys and scaling.
- GitHub Actions syncs secrets on every deploy before rolling out each app.

### One-time bootstrap

```bash
# Create apps (if they don't exist yet)
fly apps create delify-lavalink
fly apps create delify-bot

# First Lavalink deploy: private Flycast only, no public IP
fly deploy -a delify-lavalink -c fly.lavalink.toml --flycast --no-public-ips --remote-only --ha=false --wait-timeout 15m

# Bot deploy
fly deploy -a delify-bot -c fly.bot.toml --no-public-ips --remote-only --ha=false --wait-timeout 15m
```

If `delify-lavalink` ever has public IPs from an older deploy, remove them:

```bash
fly ips list -a delify-lavalink
fly ips release <public-ip> -a delify-lavalink
```

### GitHub Actions secrets to add/update

| Secret Name | Required | Used By | Notes |
|-------------|----------|---------|-------|
| `FLY_API_TOKEN` | ✅ | Fly workflow | Create with `fly tokens create deploy -x 999999h` |
| `FLY_PRIMARY_REGION` | Optional | Fly workflow | Example: `iad`, `cdg`, `sjc`; sets `--primary-region` for both apps |
| `DISCORD_TOKEN` | ✅ | bot | Discord bot token |
| `LAVALINK_PASSWORD` | ✅ | bot + lavalink | Must match on both apps |
| `SPOTIFY_CLIENT_ID` | ✅ | lavalink | Spotify API client ID |
| `SPOTIFY_CLIENT_SECRET` | ✅ | lavalink | Spotify API client secret |
| `YOUTUBE_OAUTH_REFRESH_TOKEN` | Optional | lavalink | For YouTube OAuth playback access |
| `DISCORD_GUILD_IDS` | Optional | bot | Comma-separated guild IDs |
| `DEVELOPER_USER_ID` | Optional | bot | Footer/avatar user ID |
| `RAPIDAPI_KEY` | Optional | bot | Genius lyrics via RapidAPI |
| `RAPIDAPI_HOST` | Optional | bot | Defaults to `genius-song-lyrics1.p.rapidapi.com` |
| `INACTIVITY_TIMEOUT` | Optional | bot | Seconds before auto-leave |

### Manual deploys

```bash
fly deploy -a delify-lavalink -c fly.lavalink.toml --flycast --no-public-ips --remote-only --ha=false --wait-timeout 15m
fly deploy -a delify-bot -c fly.bot.toml --no-public-ips --remote-only --ha=false --wait-timeout 15m
```

### Logs

```bash
fly logs -a delify-lavalink
fly logs -a delify-bot
```

## ☁️ GCP Deployment (Legacy)

Delify supports deployment to a Google Cloud Compute Engine VM with GitHub Actions CI/CD. On every push to `main`, the pipeline builds the bot image, pushes it to Artifact Registry, and deploys via SSH to the VM.

### Prerequisites

- GCP account with active billing
- `gcloud` CLI installed and authenticated
- GitHub repository (private or public)

### Quick Setup

1. **Create GCP resources**:
   ```bash
   # Set variables (customize as needed)
   PROJECT_ID="your-gcp-project-id"
   REGION="europe-west1"
   ZONE="europe-west1-b"
   VM_NAME="delify-vm"
   AR_REPO="delify"
   SA_NAME="delify-deployer"

   gcloud config set project $PROJECT_ID

   # Enable required APIs
   gcloud services enable \
     compute.googleapis.com \
     artifactregistry.googleapis.com \
     secretmanager.googleapis.com \
     iamcredentials.googleapis.com

   # Create Artifact Registry repository
   gcloud artifacts repositories create $AR_REPO \
     --repository-format=docker \
     --location=$REGION \
     --description="Delify Docker images"

   # Create GCE VM (e2-small: 2 vCPU, 2GB RAM — good for Lavalink + bot)
   gcloud compute instances create $VM_NAME \
     --zone=$ZONE \
     --machine-type=e2-small \
     --image-family=debian-12 \
     --image-project=debian-cloud \
     --boot-disk-size=20GB \
     --tags=delify \
     --scopes=cloud-platform \
     --metadata=startup-script='#!/bin/bash
       apt-get update && apt-get install -y docker.io docker-compose-plugin
       systemctl enable docker && systemctl start docker
       usermod -aG docker $(whoami)'
   ```

2. **Add secrets to Secret Manager**:
   ```bash
   # Helper function
   add_secret() {
     echo -n "$2" | gcloud secrets create "$1" --data-file=- 2>/dev/null || \
     echo -n "$2" | gcloud secrets versions add "$1" --data-file=-
   }

   add_secret "DISCORD_TOKEN" "your_discord_bot_token"
   add_secret "DISCORD_GUILD_IDS" "123456789,987654321"
   add_secret "DEVELOPER_USER_ID" "your_discord_user_id"
   add_secret "LAVALINK_PASSWORD" "youshallnotpass"
   add_secret "SPOTIFY_CLIENT_ID" "your_spotify_client_id"
   add_secret "SPOTIFY_CLIENT_SECRET" "your_spotify_client_secret"
   add_secret "RAPIDAPI_KEY" "your_rapidapi_key"
   add_secret "RAPIDAPI_HOST" "genius-song-lyrics1.p.rapidapi.com"
   add_secret "INACTIVITY_TIMEOUT" "30"
   add_secret "YOUTUBE_OAUTH_REFRESH_TOKEN" "your_token_or_empty"
   ```

3. **Set up Workload Identity Federation** (keyless auth for GitHub Actions):
   ```bash
   # Create a Workload Identity Pool
   gcloud iam workload-identity-pools create "github-pool" \
     --location="global" \
     --display-name="GitHub Actions Pool"

   # Create a Provider for GitHub
   gcloud iam workload-identity-pools providers create-oidc "github-provider" \
     --location="global" \
     --workload-identity-pool="github-pool" \
     --display-name="GitHub Provider" \
     --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
     --attribute-condition="assertion.repository=='YOUR_GITHUB_USERNAME/delify'" \
     --issuer-uri="https://token.actions.githubusercontent.com"

   # Create a service account
   gcloud iam service-accounts create $SA_NAME \
     --display-name="Delify GitHub Actions Deployer"

   SA_EMAIL="${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

   # Grant necessary roles
   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${SA_EMAIL}" \
     --role="roles/compute.instanceAdmin.v1"

   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${SA_EMAIL}" \
     --role="roles/artifactregistry.writer"

   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${SA_EMAIL}" \
     --role="roles/secretmanager.secretAccessor"

   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${SA_EMAIL}" \
     --role="roles/iap.tunnelResourceAccessor"

   # Allow GitHub Actions to impersonate the service account
   # Replace YOUR_GITHUB_USERNAME/delify with your actual repo
   REPO="YOUR_GITHUB_USERNAME/delify"
   POOL_ID=$(gcloud iam workload-identity-pools describe "github-pool" \
     --location="global" --format="value(name)")

   gcloud iam service-accounts add-iam-policy-binding $SA_EMAIL \
     --role="roles/iam.workloadIdentityUser" \
     --member="principalSet://iam.googleapis.com/${POOL_ID}/attribute.repository/${REPO}"

   # Get the Workload Identity Provider resource name (needed for GitHub secret)
   gcloud iam workload-identity-pools providers describe "github-provider" \
     --location="global" \
     --workload-identity-pool="github-pool" \
     --format="value(name)"
   ```

4. **Add GitHub Secrets** (Settings → Secrets → Actions):

   | Secret Name | Value |
   |-------------|-------|
   | `GCP_PROJECT_ID` | Your GCP project ID |
   | `GCP_REGION` | e.g., `europe-west1` |
   | `GCP_ZONE` | e.g., `europe-west1-b` |
   | `GCE_INSTANCE` | e.g., `delify-vm` |
   | `ARTIFACT_REGISTRY_REPO` | e.g., `delify` |
   | `GCP_WORKLOAD_IDENTITY_PROVIDER` | Full provider name from step 3 |
   | `GCP_SERVICE_ACCOUNT` | e.g., `delify-deployer@project.iam.gserviceaccount.com` |

5. **Initial VM setup** (one-time):
   ```bash
   # SSH into the VM
   gcloud compute ssh $VM_NAME --zone=$ZONE

   # On the VM: install Docker (if not done via startup script)
   # Copy and run: deploy/gcp/setup-vm.sh
   ```

6. **Push to main** — GitHub Actions will automatically deploy!

### View Logs

```bash
# SSH and view logs
gcloud compute ssh delify-vm --zone=europe-west1-b \
  --command="cd ~/delify && docker compose -f docker-compose.prod.yml logs -f"

# Bot logs only
gcloud compute ssh delify-vm --zone=europe-west1-b \
  --command="cd ~/delify && docker compose -f docker-compose.prod.yml logs -f bot"

# Lavalink logs only
gcloud compute ssh delify-vm --zone=europe-west1-b \
  --command="cd ~/delify && docker compose -f docker-compose.prod.yml logs -f lavalink"
```

### Manual Deployment (GCE VM)

1. SSH into the VM: `gcloud compute ssh delify-vm --zone=europe-west1-b`
2. Clone the repository
3. Configure `.env`
4. Run `docker compose up -d`

---

## ☁️ Azure Deployment (Legacy)

Delify supports deployment to Azure Container Apps with GitHub Actions CI/CD. On every push to `main`, the pipeline automatically builds and deploys both Lavalink and the bot.

### Prerequisites

- Azure account with active subscription
- Azure CLI installed (`az login`)
- GitHub repository (private or public)

### Quick Setup

1. **Create Azure resources**:
   ```bash
   # Set variables (customize names as needed)
   RESOURCE_GROUP="delify-rg"
   LOCATION="westeurope"
   ACR_NAME="delifyacr$(date +%s | tail -c 5)"  # Must be globally unique
   KEYVAULT_NAME="delify-kv-$(date +%s | tail -c 5)"
   CONTAINER_APP_ENV="delify-env"

   # Create resources
   az group create --name $RESOURCE_GROUP --location $LOCATION
   az acr create --resource-group $RESOURCE_GROUP --name $ACR_NAME --sku Basic --admin-enabled true
   az keyvault create --resource-group $RESOURCE_GROUP --name $KEYVAULT_NAME --location $LOCATION
   az containerapp env create --resource-group $RESOURCE_GROUP --name $CONTAINER_APP_ENV --location $LOCATION
   ```

2. **Grant yourself Key Vault access** (if using RBAC):
   ```bash
   USER_OID=$(az ad signed-in-user show --query id -o tsv)
   KV_ID=$(az keyvault show --name $KEYVAULT_NAME --query id -o tsv)
   az role assignment create --role "Key Vault Secrets Officer" --assignee $USER_OID --scope $KV_ID
   ```

3. **Add secrets to Key Vault**:
   ```bash
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "DISCORD-TOKEN" --value "your_token"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "DISCORD-GUILD-IDS" --value "123,456"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "DEVELOPER-USER-ID" --value "your_id"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "LAVALINK-PASSWORD" --value "youshallnotpass"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "SPOTIFY-CLIENT-ID" --value "your_id"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "SPOTIFY-CLIENT-SECRET" --value "your_secret"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "RAPIDAPI-KEY" --value "your_key"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "RAPIDAPI-HOST" --value "genius-song-lyrics1.p.rapidapi.com"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "INACTIVITY-TIMEOUT" --value "30"
   az keyvault secret set --vault-name $KEYVAULT_NAME --name "YOUTUBE-OAUTH-REFRESH-TOKEN" --value "your_token_or_none"
   ```

4. **Create service principal for GitHub Actions**:
   ```bash
   az ad sp create-for-rbac --name "delify-github-actions" --role contributor \
     --scopes /subscriptions/$(az account show --query id -o tsv)/resourceGroups/$RESOURCE_GROUP \
     --json-auth
   ```
   Save the JSON output for the next step.

5. **Grant service principal access to ACR and Key Vault**:
   ```bash
   SP_CLIENT_ID="<clientId from JSON above>"
   ACR_ID=$(az acr show --name $ACR_NAME --query id -o tsv)
   KV_ID=$(az keyvault show --name $KEYVAULT_NAME --query id -o tsv)
   
   az role assignment create --assignee $SP_CLIENT_ID --role AcrPush --scope $ACR_ID
   az role assignment create --assignee $SP_CLIENT_ID --role "Key Vault Secrets User" --scope $KV_ID
   ```

6. **Add GitHub Secrets** (Settings → Secrets → Actions):
   | Secret Name | Value |
   |-------------|-------|
   | `AZURE_CREDENTIALS` | Entire JSON from step 4 |
   | `ACR_NAME` | Your ACR name (e.g., `delifyacr7444`) |
   | `RESOURCE_GROUP` | `delify-rg` |
   | `KEYVAULT_NAME` | Your Key Vault name |
   | `CONTAINER_APP_ENV` | `delify-env` |

7. **Push to main** - GitHub Actions will automatically deploy!

### View Logs

```bash
# Bot logs
az containerapp logs show --name delify-bot --resource-group delify-rg --follow

# Lavalink logs  
az containerapp logs show --name delify-lavalink --resource-group delify-rg --follow
```

### Manual Deployment (Azure VM)

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
