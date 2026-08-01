# Scheduled Ingestion and Automation

NoteBrain uses native OS schedulers (for example, **cron** and **systemd service**). It does not run a background daemon that uses CPU and memory.

## Why OS Schedulers?

- **Zero idle overhead**: NoteBrain uses zero RAM and zero CPU when it does not do an index operation.
- **Fast incremental indexing**: NoteBrain calculates content hashes and word counts during ingestion. When you run NoteBrain on a schedule, it skips unmodified markdown notes in milliseconds.
- **Robustness**: System schedulers manage wake-from-sleep events, missed executions, and log rotation. They do not require custom monitoring.

## Recommended Schedule: 3-Hour Window

Run automated ingestion every 3 hours. This interval gives a good balance between fresh indexes and system performance.

## Option 1: Linux and macOS (Cron)

Cron is available on almost all Unix-like operating systems.

1. Open your crontab in your editor:
   ```bash
   crontab -e
   ```
2. Append the schedule from [contrib/automation/crontab.example](contrib/automation/crontab.example):
   ```cron
   0 */3 * * * /usr/local/bin/notebrain ingest >> ~/.notebrain/ingest.log 2>&1
   ```
   Note: Make sure that `/usr/local/bin/notebrain` matches the path from the `which notebrain` command.

## Option 2: Linux (Systemd User Timers)

Systemd user timers give precise execution tracking and low-priority execution (`Nice=19`). They also run missed executions automatically (`Persistent=true`).

The repository gives template files in [contrib/automation/systemd/](https://github.com/nmdra/notebrain-cli/tree/master/contrib/automation/systemd).

1. Create the systemd user configuration directory:
   ```bash
   mkdir -p ~/.config/systemd/user
   ```

2. Copy the service template and the timer unit template from `contrib/automation/systemd/`:
   ```bash
   cp contrib/automation/systemd/notebrain-ingest.service ~/.config/systemd/user/
   cp contrib/automation/systemd/notebrain-ingest.timer ~/.config/systemd/user/
   ```

3. Reload the systemd user units. Then enable the timer:
   ```bash
   systemctl --user daemon-reload
   systemctl --user enable --now notebrain-ingest.timer
   ```

4. See the status and the next execution times:
   ```bash
   systemctl --user list-timers --all | grep notebrain
   systemctl --user status notebrain-ingest.timer
   ```

## Logs

All configuration templates send the standard output and the error output to a unified log file:

```bash
~/.notebrain/ingest.log
```

To read live ingestion cycles, use `tail`:

```bash
tail -f ~/.notebrain/ingest.log
```
