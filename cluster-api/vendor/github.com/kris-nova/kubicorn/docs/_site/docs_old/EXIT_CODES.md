# Exit Codes

This file documents exit codes which `kubicorn` will return on errors.

| Exit code | Meaning |
|---|---|
| 1 | General error |
| 3 | Terminated — SIGKILL, SIGTERM |
| 4 | Timeout occured |
| 130 | Terminated — SIGINT |