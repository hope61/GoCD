# GoCD

Auto-pulls your git repos and builds and runs them so you don't have to. Set it up once with a cron job and forget about it.

## Install

```
git clone https://github.com/hope61/GoCD.git
cd GoCD
go build -o gocd ./cmd/gocd/
```

## Config

Create a `config.yaml` next to the binary:

```yaml
repos:
  - dir: /home/user/project-a
  - dir: /home/user/project-b
    remote: upstream
    branch: develop
    commands:
      - docker compose up -d --build
```

`remote` and `branch` are optional, they default to `origin` and `main`.

Commands only run when new changes are actually pulled, not when the repo is already up to date. They run in order and stop if one fails. You can chain commands in a single entry with `&&`:

```yaml
commands:
  - npm install && npm run build
  - systemctl restart myapp
```

Without a config file it falls back to flags:

```
./gocd --dir /path/to/repo --remote origin --branch main
```

## Cron

Open your crontab with `crontab -e` and add:

```
*/5 * * * * /path/to/gocd
```

That runs it every 5 minutes. Logs are saved to a `logs/` folder next to the binary, one file per day, and old ones get cleaned up after a week.
