# GoCD

Auto-pulls your git repos and builds and runs them so you don't have to. Set it up once as a systemd service and forget about it.

## Install

```
git clone https://github.com/hope61/GoCD.git
cd GoCD
sudo make service
```

That builds the binary, installs it to `/opt/gocd`, links it to `/usr/local/bin/gocd` so you can just run `gocd`, and starts a `gocd` service that runs as your user. Put your `config.yaml` in `/opt/gocd/` and run `sudo systemctl restart gocd` after changing it.

Other targets:

```
make build            # just the binary
sudo make install     # install without the service
make secret           # generate a webhook secret
sudo make uninstall   # remove everything
```

## Config

gocd reads `config.yaml` from the folder its binary is in, so the live config is `/opt/gocd/config.yaml`:

```
sudo cp example-config.yaml /opt/gocd/config.yaml
sudo nano /opt/gocd/config.yaml
sudo systemctl restart gocd
```

A `config.yaml` in the repo folder is only copied to `/opt/gocd` on the first install. After that, edit the one in `/opt/gocd`; changing the repo copy does nothing.

It looks like this:

```yaml
mode: poll
interval: 5m
repos:
  - dir: /home/user/project-a
  - dir: /home/user/project-b
    remote: upstream
    branch: develop
    commands:
      - docker compose up -d --build
```

`mode` is `poll` or `webhook` and defaults to `poll`. `interval` is the time between checks (`30s`, `5m`, `1h`) and defaults to `5m`. `remote` defaults to `origin`. `branch` defaults to whatever branch the repo is currently on. If you set a branch and the repo is on a different one, it logs a warning and skips that repo.

Commands only run when new changes are actually pulled. They run in order and stop if one fails. You can chain commands with `&&`:

```yaml
commands:
  - npm install && npm run build
  - systemctl restart myapp
```

## Webhook

Polling checks every `interval`. Webhook mode pulls as soon as you push instead:

```yaml
mode: webhook
listen: :9000
secret: your-secret
```

Generate a secret with `make secret`. It's saved to `webhook.secret` (ignored by git), copy it into `config.yaml`. Use the same secret on every webhook.

Set `url` in `config.yaml` to the public address GitHub should call:

```yaml
url: https://your-host:9000/
```

Then add the webhook to the repos you want (needs the `gh` CLI, logged in with `gh auth login`):

```
gocd --add-hook hope61/project-a hope61/project-b
gocd --remove-hook hope61/project-a
```

It skips repos that already have the webhook. To cover every repo in an organization at once, add it by hand under the org's Settings → Webhooks with the same url, content type `application/json`, your secret and just the push event.

Each push checks all repos in the config. It also checks once on startup.

If you can't open a port, put a tunnel like Cloudflare Tunnel, Tailscale Funnel or ngrok in front of `localhost:9000` and use its URL as `url`. Pick one with a fixed URL.

### Cloudflare Tunnel

In Cloudflare go to Zero Trust → Networks → Tunnels and add a public hostname to your tunnel:

- Hostname: `hook.yourdomain.com`
- Service: `http://localhost:9000`

Then use it in `config.yaml`, without the port since Cloudflare serves it on normal https:

```yaml
mode: webhook
listen: 127.0.0.1:9000
secret: your-secret
url: https://hook.yourdomain.com/
```

`127.0.0.1` makes gocd only reachable through the tunnel. Run `sudo systemctl restart gocd`, then `gocd --add-hook` your repos.

Make sure Bot Fight Mode, Under Attack mode or WAF challenges don't apply to that hostname, otherwise Cloudflare blocks GitHub before it reaches gocd. The secret already keeps everyone else out.

To test it, open the repo's Settings → Webhooks → your hook → Recent Deliveries. The ping should show `204`:

- `401`: the secret doesn't match, or gocd wasn't restarted after setting it
- `403` or an HTML page: Cloudflare blocked it
- `502` / `530`: the tunnel can't reach gocd, check `systemctl status gocd`

Then push a commit and watch it pull with `gocd -l -f`.

## Logs

Logs are saved to a `logs/` folder next to the binary, one file per day, and old ones get cleaned up after a week.

```
gocd -l      # last 50 lines
gocd -l -f   # follow
```
