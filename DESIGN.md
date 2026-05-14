# miniaws — Project Design

A Go CLI to spin up a [ministack](https://hub.docker.com/r/ministackorg/ministack) container and manage AWS resources (S3, SSM Parameter Store, SQS) via the local endpoint.

## Project Structure

```
miniaws/
├── main.go               # Entry point: calls cmd.Execute()
├── go.mod / go.sum       # Deps: cobra, docker client, aws-sdk-go-v2
├── DESIGN.md             # This file
├── README.md             # GitHub readme
└── cmd/
    ├── root.go           # Cobra root command; registers subcommands, global flags
    ├── init.go           # `miniaws init` — ensure container exists/running, prompt setup
    ├── config.go         # Load/Save Config to ~/.miniaws/config.json
    ├── container_cmd.go  # `miniaws container` subcommands: status, start, stop, remove
    ├── browse.go         # `miniaws browse` — interactive TUI dashboard (S3)
    ├── s3_cmd.go         # `miniaws s3` subcommands: ls, mb, rb, cp
    ├── ssm_cmd.go        # `miniaws ssm` subcommands: ls, get, put, rm
    ├── sqs_cmd.go        # `miniaws sqs` subcommands: ls, create, rm, send, recv
    └── internal/
        ├── awsclient/
        │   └── awsclient.go  # Shared AWS config + service client factories
        ├── s3ops/
        │   └── s3ops.go      # S3 operations (100% test coverage)
        ├── ssmops/
        │   └── ssmops.go     # SSM Parameter Store operations
        └── sqsops/
            └── sqsops.go     # SQS operations
```

## Commands

| CLI invocation                    | What it does                                              |
|-----------------------------------|-----------------------------------------------------------|
| `miniaws init`                    | Check container; if missing/stopped, prompt + create/start |
| `miniaws browse`                  | Interactive TUI dashboard (container + S3 buckets)        |
| `miniaws s3 ls`                   | List all S3 buckets                                       |
| `miniaws s3 ls <bucket/prefix>`   | List objects in a bucket                                  |
| `miniaws s3 mb <bucket>`          | Create an S3 bucket                                       |
| `miniaws s3 rb <bucket>`          | Remove an S3 bucket (fails if not empty)                  |
| `miniaws s3 rb <bucket> -f`       | Force remove: empty bucket first, then delete             |
| `miniaws s3 cp <src> <dst>`       | Upload/download files (local ↔ s3://)                     |
| `miniaws ssm ls`                  | List all parameters                                       |
| `miniaws ssm get <name>`          | Get parameter value                                       |
| `miniaws ssm put <name> <val>`    | Create/update parameter                                   |
| `miniaws ssm rm <name>`           | Delete parameter                                          |
| `miniaws sqs ls`                  | List all queues (paginated)                               |
| `miniaws sqs create <name>`       | Create a queue                                            |
| `miniaws sqs rm <url>`            | Delete a queue                                            |
| `miniaws sqs send <url> <msg>`    | Send a message                                            |
| `miniaws sqs recv <url>`          | Receive messages                                          |
| `miniaws container status`        | Show container state                                      |
| `miniaws container start`         | Start the configured container                            |
| `miniaws container stop`          | Stop the configured container                             |
| `miniaws container remove`        | Remove container and delete config file                   |
| `miniaws container remove -f`     | Force remove even if running                              |

Global flags:

| Flag                     | Default                  | Description              |
|--------------------------|--------------------------|--------------------------|
| `--endpoint-url`         | `http://localhost:4566`  | Ministack API endpoint   |

All non-Docker commands work offline. Docker commands gracefully report when not initialized.

## Data Flow

### Init (`cmd/init.go`)
1. Connect to Docker daemon (`client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())`)
2. Load config from `~/.miniaws/config.json`
3. If config exists → inspect container by name
   - Running → print message, exit
   - Exited → start it, exit
   - Not found → fall through to setup
4. If no config → prompt user: container name, image, endpoint URL
5. Call `ensureContainer`:
   - Inspect container → if exists start
   - If not found → `ImagePull` then `ContainerCreate` then `ContainerStart`
   - If pull fails → print warning, prompt for different image name, retry
6. Save config

### Shared AWS config + client factories (`internal/awsclient/awsclient.go`)

- `NewConfig()` — returns `aws.Config` with region `us-east-1`, dummy static credentials, single retry attempt (fail fast)
- `NewS3Client(cfg, endpoint)` — S3 client with path-style addressing
- `NewSSMClient(cfg, endpoint)` — SSM client with base endpoint
- `NewSQSClient(cfg, endpoint)` — SQS client with base endpoint

The cmd layer creates the config once, then passes it to the appropriate factory. Each service package receives an already-configured client. No service package imports `awsclient`.

### Service packages (`internal/s3ops/`, `internal/ssmops/`, `internal/sqsops/`)

Each is a collection of stateless functions taking the injected client as the first argument. Pattern:

```go
func ListBuckets(ctx context.Context, client *s3.Client) ([]string, error)
func ListAllParameters(ctx context.Context, client *ssm.Client) ([]Parameter, error) // CLI: all pages
func ListParameters(ctx context.Context, client *ssm.Client, nextToken *string, maxResults int32) (*Page, error) // TUI: one page
func ListQueues(ctx context.Context, client *sqs.Client) ([]Queue, error)
```

Two pagination strategies:
- **CLI commands** use `ListAllParameters` / S3 paginators / SQS paginators — all pages collected into a single result slice.
- **TUI** (SSM only) uses `ListParameters` with a page size of 20. The model tracks `requestToken`, `nextToken`, and a `prevTokens` stack for forward/backward page navigation via `[` / `]` keys.

### Container management (`cmd/container_cmd.go`)
All commands load config first. If no config → print "Not initialized. Run 'miniaws init' first."
- `status`: inspect container, print name/image/state
- `start`: `ContainerStart` (fails if container doesn't exist)
- `stop`: `ContainerStop`
- `remove`: `ContainerRemove` then delete config file; supports `--force`

## Conventions

- All source files in `cmd/` are in `package cmd` (flat package).
- Errors bubble up via `RunE` returning them; Cobra prints the error.
- User-facing output uses `fmt.Print` directly (no logger).
- Variables shadowing package names are avoided — use `ci`, `resp`, etc.
- Config is stored under `~/.miniaws/config.json`.
- Docker API: `container.StartOptions{}`, `image.PullOptions{}`, `container.InspectResponse`.
- AWS SDK v2: clients created via `awsclient` factories with dummy credentials.
- Each service package has `IsConnectionErr` and `friendlyErr` helpers for consistent error messages.

### Browse TUI (`cmd/browse.go`)

- Full-screen Bubble Tea app with alt screen
- **Service switcher** at top — press `[1]` for S3, `[2]` for SSM
- Dashboard sections: container status, then active service panel

**S3 mode:**
- Bucket list (default) → `enter` to browse objects
- Object view: `u` upload, `d` download, `del` delete, `esc` back

**SSM mode:**
- Parameter list, loaded one page at a time (20 items per page)
- `[` / `←` previous page, `]` / `→` next page
- `enter` fetches and displays the parameter value in the status line
- `del` / `backspace` deletes the selected parameter

**General keybindings:**
  - `↑`/`k` `↓`/`j` — navigate current list
  - `r` — refresh all data
  - `s` — start container (when not running)
  - `x` — stop container (when running)
  - `q` / `ctrl+c` — quit
  - `esc` — go back / quit
- Action feedback displayed below section; dashboard auto-refreshes after action

## Visual Design

Output is styled with [lipgloss](https://github.com/charmbracelet/lipgloss):
- **Bold blue** labels and bucket/queue/parameter names
- **Green bold** for success messages and running status
- **Yellow bold** for warnings and non-running states
- **Grey italic** for empty state messages, sizes, types
- **Cyan** for folder/prefix listings
- Emoji indicators: 📦 bucket/queue, 📁 folder, 📄 object

## Test Coverage

| Package | Tests | Coverage |
|---------|-------|----------|
| `internal/awsclient/` | 8 | Config, credentials, retryer, 3 client factories |
| `internal/s3ops/` | 28 | **100%** — S3 CRUD, error handling, pagination |
| `internal/ssmops/` | 20 | **100%** — SSM CRUD, pagination, error handling |
| `internal/sqsops/` | 0 | TBD |
