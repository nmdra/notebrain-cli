# Scheduled Ingestion and Automation

NoteBrain uses native OS schedulers such as **cron** and **systemd timers**. The [MCP server](MCP.md) handles retrieval only. It does not index the vault or schedule ingestion.

## Why OS Schedulers?

- **No indexing daemon**: The scheduler starts a process for each ingestion run.
- **Fast incremental indexing**: NoteBrain calculates content hashes and word counts during ingestion. When you run NoteBrain on a schedule, it skips unmodified markdown notes in milliseconds.
- **Missed runs and wake-from-sleep**: System schedulers manage wake-from-sleep events, missed executions, and log rotation. They do not require custom monitoring.

## Recommended Schedule: 3-Hour Window

Run automated ingestion every 3 hours. This interval gives a good balance between fresh indexes and system performance.

## Shared Stores With MCP

Use the same current NoteBrain version and the same user account for scheduled ingestion and MCP clients that share a store. These processes take the same advisory lock for the configured `--chroma-path`. Old NoteBrain versions and direct Chroma clients do not take that lock.

Ingestion can make retrieval calls wait. An MCP call has a 90-second deadline that includes waiting for the lock. If ingestion regularly exceeds that deadline, move the scheduled run outside busy retrieval periods. Do not delete the lock file while any client or job can use the store.

## Option 1: Linux (Cron)

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
