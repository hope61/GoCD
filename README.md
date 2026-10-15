# GoCD

Auto-pulls your git repos so you don't have to. Set it up once with a cron job and forget about it.

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
```

`remote` and `branch` are optional, they default to `origin` and `main`.

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
