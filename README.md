# miniaws

A CLI utility to spin up a [ministack](https://hub.docker.com/r/ministackorg/ministack) container (free LocalStack alternative) and manage AWS resources via the local endpoint.

## Requirements

- Go 1.26+
- Docker (for `init`, `container start/stop/status/remove`)

### Nix / direnv (optional)

A `flake.nix` is provided for a reproducible development shell:

```bash
# If you use direnv:
direnv allow

# Or enter the shell manually:
nix develop
```

This provides Go 1.26, `gopls`, and `golangci-lint` without installing them globally.

## Install

```bash
go install github.com/syasika/miniaws@latest
```

Or build from source:

```bash
git clone https://github.com/syasika/miniaws
cd miniaws
go build -o miniaws .
```

## Quick Start

```bash
# Start a ministack container (interactive — prompts for name & image)
miniaws init

# S3 — list buckets, create, upload, download
miniaws s3 ls
miniaws s3 mb my-bucket
miniaws s3 cp ./photo.jpg s3://my-bucket/photo.jpg

# SSM Parameter Store — list, get, put, delete
miniaws ssm ls
miniaws ssm put /config/db-url "localhost"
miniaws ssm get /config/db-url

# SQS — list queues, create, send, receive
miniaws sqs ls
miniaws sqs create my-queue
miniaws sqs send http://localhost:4566/queue/my-queue "hello"

# Container lifecycle
miniaws container status
miniaws container stop
miniaws container remove
```

## Commands

| Command                         | Description                                |
|---------------------------------|--------------------------------------------|
| `miniaws init`                  | Ensure container exists; prompt if needed  |
| `miniaws browse`                | Interactive TUI dashboard (S3 + SSM)       |
| `miniaws s3 ls [bucket]`        | List buckets or objects                    |
| `miniaws s3 mb <bucket>`        | Create an S3 bucket                        |
| `miniaws s3 rb <bucket>`        | Remove an S3 bucket                        |
| `miniaws s3 rb <bucket> -f`     | Force remove (empty then delete)           |
| `miniaws s3 cp <src> <dst>`     | Upload/download files (local ↔ s3://)      |
| `miniaws ssm ls`                | List SSM parameters                        |
| `miniaws ssm get <name>`        | Get parameter value                        |
| `miniaws ssm put <name> <val>`  | Create/update a parameter                  |
| `miniaws ssm rm <name>`         | Delete a parameter                         |
| `miniaws sqs ls`                | List queues                                |
| `miniaws sqs create <name>`     | Create a queue                             |
| `miniaws sqs rm <url>`          | Delete a queue                             |
| `miniaws sqs send <url> <msg>`  | Send a message                             |
| `miniaws sqs recv <url>`        | Receive messages                           |
| `miniaws container status`      | Show container state                       |
| `miniaws container start`       | Start the container                        |
| `miniaws container stop`        | Stop the container                         |
| `miniaws container remove`      | Remove the container and reset config      |

Global flags:

| Flag                     | Default                  | Description              |
|--------------------------|--------------------------|--------------------------|
| `--endpoint-url`         | `http://localhost:4566`  | Ministack API endpoint   |

## Browse TUI

Launch with `miniaws browse`. The dashboard shows the container status at the top, then the active service panel.

**Service switcher** — press `[1]` for S3, `[2]` for SSM Parameter Store, `[3]` for SQS.

Destructive actions (delete object, delete parameter, delete queue, delete message, stop container) prompt for `y`/`N` confirmation before executing.

| View | Controls |
|------|----------|
| **S3 buckets** | `↑`/`↓` navigate, `enter` to browse objects |
| **S3 objects** | `u` upload, `d` download, `del` delete (with confirmation), `esc` back |
| **SSM params** | `↑`/`↓` navigate, `enter` for value, `del` delete (with confirmation), `[`/`]` page |
| **SQS queues** | `↑`/`↓` navigate, `enter` for messages, `c` create, `del` delete (with confirmation) |
| **SQS messages** | `↑`/`↓` navigate, `s` send, `del` delete (with confirmation), `esc` back |

## Storage

- **Config**: `~/.miniaws/config.json` — container name, image, port, endpoint URL

## License

MIT
