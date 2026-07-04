# Scheduled Ingestion & Automation

Rather than running a persistent background watch daemon that consumes idle CPU and memory, NoteBrain is designed to be executed periodically by native OS schedulers like **cron**, **systemd service**.

## Why OS Schedulers?

1. **Zero Idle Overhead**: When NoteBrain isn't actively indexing, it consumes zero RAM and zero CPU.
2. **Fast Incremental Indexing**: NoteBrain automatically calculates content hashes and word counts during ingestion. When executed on a schedule, unmodified markdown notes are skipped in milliseconds.
3. **Robustness & Catch-Up**: System schedulers automatically handle wake-from-sleep events, missed executions, and log rotation without requiring custom daemon monitoring.

## Recommended Schedule: 3-Hour Window

We recommend running automated ingestion **every 3 hours**. This provides an optimal balance between index freshness and background system impact.

## Option 1: Linux & macOS (Cron)

Cron is universally available on almost all Unix-like operating systems.

1. Open your crontab in your editor:
   ```bash
   crontab -e
   ```
2. Append the schedule from [contrib/automation/crontab.example](contrib/automation/crontab.example) (running every 3 hours):
   ```cron
   0 */3 * * * /usr/local/bin/notebrain ingest >> ~/.notebrain/ingest.log 2>&1
   ```
   _(Ensure `/usr/local/bin/notebrain` matches the absolute path returned by `which notebrain`)._

## Option 2: Linux (Systemd User Timers)

Systemd user timers offer precise execution tracking, automatic catch-up for missed runs (`Persistent=true`), and low priority execution (`Nice=19`).

Template files are provided in the repository under [contrib/automation/systemd/](https://github.com/nmdra/notebrain-cli/tree/master/contrib/automation/systemd).

1. Create the systemd user configuration directory:
   ```bash
   mkdir -p ~/.config/systemd/user
   ```
2. Copy the service and timer unit templates from `contrib/automation/systemd/`:
   ```bash
   cp contrib/automation/systemd/notebrain-ingest.service ~/.config/systemd/user/
   cp contrib/automation/systemd/notebrain-ingest.timer ~/.config/systemd/user/
   ```
3. Reload systemd user units and enable the timer:
   ```bash
   systemctl --user daemon-reload
   systemctl --user enable --now notebrain-ingest.timer
   ```
4. Check the status and upcoming execution times:
   ```bash
   systemctl --user list-timers --all | grep notebrain
   systemctl --user status notebrain-ingest.timer
   ```

## Monitoring Logs

All configuration templates redirect standard output and error to a unified log file:

```bash
~/.notebrain/ingest.log
```

You can follow live ingestion cycles using `tail`:

```bash
tail -f ~/.notebrain/ingest.log
```
