# GoCD

Auto-pulls your git repos and builds and runs them so you don't have to. Set it up once as a systemd service and forget about it.

## Install

```
git clone https://github.com/hope61/GoCD.git
cd GoCD
sudo make service
```

That builds the binary, installs it to `/opt/gocd`, and starts a `gocd` systemd service that runs as your user. Put your `config.yaml` in `/opt/gocd/` (or next to the Makefile before installing) and run `sudo systemctl restart gocd` after changing it.

Other targets: `make build` (just the binary), `sudo make install` (binary only, no service), `sudo make uninstall`.

## Config

Create a `config.yaml` next to the binary:

```yaml
interval: 5m
repos:
  - dir: /home/user/project-a
  - dir: /home/user/project-b
    remote: upstream
    branch: develop
    commands:
      - docker compose up -d --build
```

`interval` is the time between checks (`30s`, `5m`, `1h`) and defaults to `5m`. `remote` defaults to `origin`. `branch` defaults to whatever branch the repo is currently on. If you set a branch and the local repo is on a different one, it logs a warning and skips that repo.

Commands only run when new changes are actually pulled, not when the repo is already up to date. They run in order and stop if one fails. You can chain commands in a single entry with `&&`:

```yaml
commands:
  - npm install && npm run build
  - systemctl restart myapp
```

## Logs

Logs are saved to a `logs/` folder next to the binary, one file per day, and old ones get cleaned up after a week.

```
/opt/gocd/gocd -l      # last 50 lines
/opt/gocd/gocd -l -f   # follow
```
