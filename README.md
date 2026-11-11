# GoCD

GoCD keeps the apps on your server up to date. You push to GitHub. GoCD pulls the new code and runs your build and restart commands. You don't need to log in to the server for every change.

- **Small.** One Go binary and one config file.
- **Two modes.** Check every few minutes (poll), or get the change right when you push (webhook).
- **Safe.** It only runs your commands when there is new code. A stuck command gets killed after a time limit.
- **Hands-off.** It runs as a systemd service and starts again if it crashes.

> [!NOTE]
> GoCD needs Linux with systemd, `git` and Go 1.26 or newer to build. Webhook setup also uses the [`gh`](https://cli.github.com) CLI.

## Contents

- [Install](#install)
- [Config](#config)
- [Webhook mode](#webhook-mode)
- [Cloudflare Tunnel](#cloudflare-tunnel)
- [Logs](#logs)
- [Commands](#commands)
- [Project layout](#project-layout)

## Install

```
git clone https://github.com/hope61/GoCD.git
cd GoCD
sudo make service
```

This does four things:

1. It builds the `gocd` binary.
2. It copies it to `/opt/gocd`.
3. It links it to `/usr/local/bin/gocd`, so you can run `gocd` from anywhere.
4. It starts a `gocd` service. The service runs as your user, so it uses your git keys.

Next, [write your config](#config).

## Config

GoCD reads `config.yaml` from the same folder as the binary. So the live config is `/opt/gocd/config.yaml`.

```
cp example-config.yaml /opt/gocd/config.yaml
gocd -e
sudo systemctl restart gocd
```

> [!IMPORTANT]
> Always edit the config in `/opt/gocd`. A `config.yaml` in the repo folder only gets copied on the first install. After that, changes to it do nothing.

### A simple config

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

### All options

| Option | Default | What it does |
|---|---|---|
| `mode` | `poll` | `poll` or `webhook` |
| `interval` | `5m` | Time between checks in poll mode, like `30s`, `5m` or `1h` |
| `timeout` | `30m` | Kill a command that runs longer than this |
| `listen` | `:9000` | Address for webhooks |
| `secret` | | Webhook secret. You need it in webhook mode |
| `url` | | Public webhook URL, used by `--add-hook` |

Each repo can have:

| Option | Default | What it does |
|---|---|---|
| `dir` | | Full path to the repo. You need this one |
| `remote` | `origin` | Git remote to pull from |
| `branch` | current branch | If the repo is on another branch, GoCD skips it and logs a warning |
| `mode` | top-level `mode` | Use a different mode for just this repo |
| `commands` | | Commands to run after new code is pulled |

### Commands

GoCD runs the commands only when it pulls new code. They run in order. If one fails, the rest don't run. You can join commands with `&&`:

```yaml
commands:
  - npm install && npm run build
  - systemctl restart myapp
```

If a command runs longer than `timeout`, GoCD kills it and everything it started. Because of this, one stuck build can't block your other repos.

### Edit with `gocd -e`

`gocd -e` works like `crontab -e`. It opens the config in your `$EDITOR`. If you don't have one, it lets you pick nvim, vim or nano. Then it saves your pick in `~/.bashrc` or `~/.zshrc`.

When you close the editor, GoCD checks the config. It only saves the config if it has no errors. If it has errors, GoCD shows them and lets you fix them.

## Webhook mode

In poll mode, GoCD checks every `interval`. In webhook mode, GitHub tells GoCD about a push, so GoCD pulls right away.

**1. Make a secret.**

```
make secret
```

This saves a secret to `webhook.secret`. Git ignores this file. Use the same secret for every repo.

**2. Add it to the config.**

```yaml
mode: webhook
listen: :9000
secret: your-secret
url: https://your-host:9000/
```

`url` is the public address that GitHub calls.

**3. Add the webhook to your repos.** First, log in with `gh auth login`. Then run:

```
gocd --add-hook hope61/project-a hope61/project-b
gocd --remove-hook hope61/project-a
```

GoCD skips repos that already have the webhook.

> [!TIP]
> Do you use a GitHub organization? Then you can add one webhook for all its repos. Go to the org's **Settings → Webhooks**. Use the same url and secret, content type `application/json`, and only the push event.

GoCD checks all webhook repos on each push. It also checks every repo when it starts, and once every 24 hours. Because of this, a lost webhook (for example, the tunnel was down) still gets pulled within a day.

### Mix poll and webhook

Each repo can set its own `mode`. This helps with repos that you can't add a webhook to. For example, GoCD can update itself by polling its own repo:

```yaml
mode: webhook
interval: 5m
repos:
  - dir: /home/user/my-app
  - dir: /home/user/GoCD
    mode: poll
    commands:
      - sudo make service
```

Each mode only checks its own repos. However, checks still run one at a time. So a slow build in one repo can delay the next check of another repo.

> [!NOTE]
> Can't open a port on your server? Then put a tunnel in front of GoCD, like Cloudflare Tunnel, Tailscale Funnel or ngrok. Use the tunnel's address as `url`. Pick a tunnel with an address that doesn't change.

## Cloudflare Tunnel

**1. Add a public hostname.** In Cloudflare, go to **Zero Trust → Networks → Tunnels**. Add a public hostname to your tunnel:

- Hostname: `hook.yourdomain.com`
- Service: `http://localhost:9000`

**2. Update the config.** Leave the port out of `url`, because Cloudflare uses normal https:

```yaml
mode: webhook
listen: 127.0.0.1:9000
secret: your-secret
url: https://hook.yourdomain.com/
```

With `127.0.0.1`, people can only reach GoCD through the tunnel.

> [!WARNING]
> Does cloudflared run in Docker? Then `localhost` points to the container, not to your server. Use your server's LAN IP in both places instead: Service `http://192.168.0.10:9000` and `listen: 192.168.0.10:9000`.

**3. Restart and add the hooks.**

```
sudo systemctl restart gocd
gocd --add-hook hope61/project-a
```

> [!CAUTION]
> Bot Fight Mode, Under Attack mode and WAF challenges can block GitHub. Turn them off for this hostname. The secret already keeps other people out.

**4. Test it.** In the repo, open **Settings → Webhooks →** your hook **→ Recent Deliveries**. The ping should show `204`.

| Code | What it means |
|---|---|
| `204` | It works |
| `401` | The secret is wrong, or you didn't restart GoCD after you set it |
| `403` or an HTML page | Cloudflare blocked it |
| `502` / `530` | The tunnel can't reach GoCD. Check `systemctl status gocd` and the Docker warning above |

Then push a commit and watch it with `gocd -l -f`.

## Logs

GoCD saves logs in a `logs/` folder next to the binary. It makes one file per day and deletes files older than a week.

```
gocd -l      # last 50 lines
gocd -l -f   # follow live
```

## Commands

| Command | What it does |
|---|---|
| `gocd -l` | Show the last 50 log lines |
| `gocd -l -f` | Follow the logs live |
| `gocd -e` | Edit the config safely |
| `gocd --add-hook owner/repo ...` | Add the webhook to repos |
| `gocd --remove-hook owner/repo ...` | Remove the webhook from repos |
| `make build` | Only build the binary |
| `sudo make install` | Install without the service |
| `sudo make service` | Install and start the service |
| `make secret` | Make a webhook secret |
| `make bench` | Run the poll vs webhook test |
| `sudo make uninstall` | Remove everything |

## Project layout

| Path | What it is |
|---|---|
| `cmd/gocd/` | The main program, flags and `gocd -e` |
| `internal/config/` | Reads and checks `config.yaml` |
| `internal/git/` | Pulls repos and runs commands |
| `internal/webhook/` | Takes webhooks from GitHub and checks the secret |
| `internal/github/` | Adds and removes webhooks with `gh` |
| `internal/logger/` | Daily log files |
| `tests/` | Speed test of poll vs webhook. Results are in `tests/results.md` |
| `example-config.yaml` | A config with every option explained |
| `Makefile` | Build, install and service setup |

## Contributing

Open an issue or a pull request. To try your changes, run `make build`, then run `./gocd` next to a test `config.yaml`.

> [!NOTE]
> GoCD uses `Setpgid` to kill stuck commands. This only works on Linux and macOS, so GoCD doesn't build on Windows.
