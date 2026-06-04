# devil
App runner which restarts a process when its binary is recompiled.

## Installation
```
go install github.com/Urethramancer/devil@latest
```

## Usage
Run a server named `app` with arguments `serve` and keep watching it:
```
devil app serve
```

Load environment variables from a file:
```
devil -e envfile app serve
```

### Environment file format
The env file uses `KEY=VALUE` syntax, one per line. Lines starting with `#` or `;` are comments. Blank lines are ignored. Keys are case-sensitive.

```
# Database
DATABASE_URL=postgres://user:pass@localhost/db
; Redis
REDIS_HOST=localhost
REDIS_PORT=6379
```
