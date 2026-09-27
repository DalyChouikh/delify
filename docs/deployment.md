# Free hosting and deployment

Checked against provider documentation on 2026-09-27. Account eligibility, quotas, prices and capacity can change. No cloud resource has been provisioned by this repair. Oracle signup failed for the user and is no longer the selected route.

## No-subscription option: existing hardware

The existing Docker deployment has passed live YouTube playback and the user confirmed audible audio. Keeping it on an existing computer, or moving it to another machine you own or are authorized to use, avoids a hosting subscription. Electricity and internet are still costs, and the machine must remain powered on and connected. A university/club machine requires its owner's permission.

## Cloud options with explicit limits

| Option | Verified offer | Limitation |
| --- | --- | --- |
| [Azure for Students](https://azure.microsoft.com/en-us/free/students) | $100 credit usable within 12 months; no credit card; annual renewal while eligible | Full-time university student eligibility must be verified. Educational/development-use conditions apply. Credits and service allowances are limited; the subscription is disabled after credit exhaustion unless upgraded. This is not a promise of permanent free production hosting. |
| [AWS Free account plan](https://aws.amazon.com/free/free-tier-faqs/) | $100 signup credit, up to $100 additional earned credit | New AWS customers only; a valid payment method is required. The free plan ends after six months or credit exhaustion, whichever comes first. Resources become inaccessible at expiry unless the account is upgraded; arrange backups beforehand. |

If student eligibility is available, check Azure for Students first. Otherwise, AWS is a temporary option, not a permanent free host. Before creating anything, check the actual account offer and the full VM, disk, IP, and network costs. Do not upgrade to paid billing just to get past an eligibility error without explicitly accepting the charges.

## Why the other free tiers are not a verified fit

| Provider | Limitation |
| --- | --- |
| Fly.io | Its [free trial](https://docs.fly.io/about/free-trial/) is limited; it is not a permanent free VM offer for new deployments |
| Google Cloud | The [Compute free tier](https://docs.cloud.google.com/free/docs/free-cloud-features) does not make every associated resource free. A standard VM's external IPv4 costs $0.005/hour after one free hour/month under [VPC pricing](https://cloud.google.com/vpc/network-pricing). An ordinary public-IPv4 deployment is therefore not an ongoing zero-cost setup. IPv6-only voice playback has not been tested. |
| Render | [Free web services](https://render.com/docs/free) spin down after 15 minutes without inbound traffic; free background workers are not included. |
| Koyeb | The [free instance](https://www.koyeb.com/docs/reference/instances#free-instances) has 512 MB RAM and 0.1 vCPU, cannot be a Worker Service, and scales to zero after one hour without traffic. |
| Railway | The [free plan](https://railway.com/pricing) provides $1/month resource credit after its $5/30-day trial. Continuous operation of this two-service stack within that allowance has not been demonstrated. |

Delify needs two long-running processes, outgoing HTTPS/WebSocket connections, and outgoing Discord voice UDP. A free HTTP endpoint alone is insufficient. The music provider must also accept requests from the VM's IP; local playback does not prove that a cloud IP will work.

## Publishing and deployment safety

Repairs are published on `fix/reliable-music-playback`. The existing Fly workflow deploys on pushes to `main`; publishing the repair branch runs CI without triggering that workflow. Do not merge to `main` until its deployment behavior is intentionally approved. GCP/Azure/Fly files are legacy examples, not verified deployments or assurances of free hosting.

## Deploy to an approved host

1. Choose an existing machine or an eligible VM after confirming account access and costs. For a new VM, 2 GB RAM is a starting allocation, not a measured minimum or performance guarantee: Compose limits Lavalink to 1 GB and the bot to 256 MB, and the host needs headroom. AMD64 and ARM64 builds are supported; only AMD64 has been tested locally. Restrict inbound SSH to your IP. Permit outbound HTTPS, DNS and Discord voice UDP. Do not expose Lavalink or add inbound bot ports.
2. Install Docker Engine and the Compose plugin using [Docker's Ubuntu instructions](https://docs.docker.com/engine/install/ubuntu/), and enable Docker at boot. Using `sudo docker` is sufficient.
3. On a new host, clone the published repair branch using your normal GitHub authentication if required:

   ```sh
   git clone --branch fix/reliable-music-playback https://github.com/DalyChouikh/delify.git
   cd delify
   git rev-parse HEAD
   ```

   Record the exact commit used for deployment. Transfer `.env` separately over SSH or configure it on the host; never commit it. Restrict it with `chmod 600 .env`. Preserve the existing secret file if reusing the local deployment.
4. In the VM's project directory:

   ```sh
   sudo docker compose config --quiet
   sudo docker compose build bot
   sudo docker compose up -d --wait lavalink
   ```

5. Stop the local bot before starting the cloud bot so two instances do not control the same application:

   ```sh
   # Locally:
   docker compose stop bot
   # On the VM:
   sudo docker compose up -d --wait bot
   sudo docker compose ps
   sudo docker compose logs --tail=80 bot
   ```

6. Confirm audible YouTube search/link playback. Test pause/resume, queue, skip and stop. Spotify is explicitly deferred; if resumed later, verify eligible credentials and repeat its playback tests. Check logs for exceptions and `sudo docker stats --no-stream` during playback. Redact credentials before sharing logs.
7. Reboot the host and repeat playback verification. Monitor provider usage and credit/offer expiry. Back up credentials securely; queues are intentionally ephemeral.

## Rollback

If cloud voice/source access fails, stop the cloud bot and restart the local bot with `docker compose up -d bot`. Preserve the last working Docker image before rebuilding remotely. Test source changes in voice before deploying widely.

Actual remote deployment requires an approved host and accessible SSH destination. Cloud deployment also requires an eligible account. Use your normal authenticated cloud/SSH tools and local secret file; do not paste private keys, account passwords or tokens into chat.
