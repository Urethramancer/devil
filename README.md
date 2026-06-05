# devil

Process supervisor for Go development. Watches a binary file and restarts it on recompile. Ships with a web dashboard for inspection, log viewing, environment management, and build control.

## Installation

```
go install github.com/Urethramancer/devil@latest
```

## Usage

```
devil [flags] <program> [args...]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-e`, `--envfile` | _(none)_ | File containing `KEY=VALUE` environment variables |
| `-p`, `--port` | `48128` | Web dashboard port |
| `-V`, `--version` | | Print version and exit |

### Environment file format

`KEY=VALUE` pairs, one per line. `#` and `;` start comments. Blank lines are ignored. Keys are case-sensitive.

### Web dashboard

Devil starts a web server on `http://localhost:48128` (customisable with `-p`):

- **Status bar** — PID, uptime, restart count, process state
- **Log viewer** — live streaming log output with filter and auto-scroll
- **Environment editor** — view current vars, edit pending changes side-by-side, apply & restart
- **Build & restart** — define a build command, run it, stream output, restart on success
- **Signal controls** — send SIGHUP, SIGUSR1, SIGUSR2 to the child process
- **Screenshot protection** — values containing credential patterns (KEY, PASSWORD, SECRET, TOKEN, etc.) are masked by default; toggle to reveal; URL credentials are obfuscated (`postgres://***:***@localhost/db`)

### Build from source

```
make build      # builds with embedded git version
make install    # installs to $GOPATH/bin
```

## License

MIT
