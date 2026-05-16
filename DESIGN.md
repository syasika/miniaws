# miniaws — Project Design

A Go CLI to spin up a [ministack](https://hub.docker.com/r/ministackorg/ministack) container and manage AWS resources (S3, SSM Parameter Store, SQS) via the local endpoint.

## Project Structure

```
miniaws/
├── main.go               # Entry point: calls cmd.Execute()
├── go.mod / go.sum       # Deps: cobra, docker client, aws-sdk-go-v2
├── DESIGN.md             # This file
├── README.md             # GitHub readme
├── cmd/
│   ├── root.go           # Cobra root command; registers subcommands, global flags
│   ├── init.go           # `miniaws init` — ensure container exists/running, prompt setup
│   ├── s3/
│   │   └── cmd.go        # `miniaws s3` subcommands: ls, mb, rb, cp
│   ├── sqs/
│   │   └── cmd.go        # `miniaws sqs` subcommands: ls, create, rm, send, recv
│   ├── ssm/
│   │   └── cmd.go        # `miniaws ssm` subcommands: ls, get, put, rm
│   ├── container/
│   │   └── cmd.go        # `miniaws container` subcommands: status, start, stop, remove
│   └── browse/
│       ├── browse.go     # `miniaws browse` — model, Init/Update/View, key handling
│       ├── view.go       # TUI dashboard rendering (dashboardView)
│       ├── messages.go   # Bubble Tea message types
│       ├── container.go  # Docker container operations for TUI
│       ├── s3.go         # S3 operations for TUI
│       ├── ssm.go        # SSM operations for TUI
│       └── sqs.go        # SQS operations + fetchCurrentView
└── internal/
    ├── config/
    │   └── config.go     # Shared Config struct, LoadConfig/SaveConfig/RemoveConfig
    ├── awsclient/
    │   └── awsclient.go  # Shared AWS config + service client factories + IsConnectionErr/FriendlyErr
    ├── s3ops/
    │   └── s3ops.go      # S3 operations (100% statement coverage)
    ├── ssmops/
    │   └── ssmops.go     # SSM Parameter Store operations (100% statement coverage)
    └── sqsops/
        └── sqsops.go     # SQS operations (100% statement coverage)
```

## Commands

| CLI invocation                    | What it does                                              |
|-----------------------------------|-----------------------------------------------------------|
| `miniaws init`                    | Check container; if missing/stopped, prompt + create/start |
| `miniaws browse`                  | Interactive TUI dashboard (container + S3 + SSM + SQS)    |
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
2. Load config from `~/.miniaws/config.json` via `internal/config.LoadConfig()`
3. If config exists → inspect container by name
   - Running → print message, exit
   - Exited → start it, exit
   - Paused → unpause it, exit
   - Not found → fall through to setup
4. If no config → prompt user: container name, image
5. Call `ensureContainer`:
   - Inspect container → if exists start
   - If not found → `ImagePull` then `ContainerCreate` then `ContainerStart`
   - If pull fails → print warning, prompt for different image name, retry
6. Save config via `internal/config.SaveConfig()`

### Config (`internal/config/config.go`)
- `Config` struct: `ContainerName`, `ImageName`, `Port`, `EndpointURL`
- `LoadConfig()` — reads and deserializes `~/.miniaws/config.json`
- `SaveConfig(cfg)` — serializes and writes config file
- `RemoveConfig()` — deletes config file on container remove

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

### Container management (`cmd/container/cmd.go`)
All commands load config first. If no config → print "Not initialized. Run 'miniaws init' first."
- `status`: inspect container, print name/image/state
- `start`: `ContainerStart` (fails if container doesn't exist)
- `stop`: `ContainerStop`
- `remove`: `ContainerRemove` (supports `--force`), then `config.RemoveConfig()`

The exported function `InspectContainer(ctx, cli, name)` is shared with `cmd/init.go` for checking container state during initialization.

### Sub-command packages (`cmd/s3/`, `cmd/sqs/`, `cmd/ssm/`, `cmd/container/`)

Each is a standalone `package` (e.g. `package s3`) with an exported `Cmd() *cobra.Command` constructor. They are wired into `cmd/root.go` via:

```go
rootCmd.AddCommand(s3.Cmd())
rootCmd.AddCommand(sqs.Cmd())
rootCmd.AddCommand(ssm.Cmd())
rootCmd.AddCommand(container.Cmd())
```

Service-specific style variables (lipgloss styles) are duplicated per sub-package rather than shared — each has 3–5 lines for label/value/success/warn/error styles.

### Browse TUI (`cmd/browse/`)

The browse TUI is in `package browse` (7 files):

| File | Role |
|------|------|
| `browse.go` | Model struct, `Cmd()` constructor, `Init`/`Update`/`View`, key handling |
| `view.go` | `dashboardView()` rendering with lipgloss |
| `messages.go` | All `tea.Msg` type definitions |
| `container.go` | Docker container fetch/start/stop |
| `s3.go` | S3 bucket/object fetch/upload/download/delete |
| `ssm.go` | SSM parameter list/get/delete |
| `sqs.go` | SQS queue/message ops + `fetchCurrentView` — dispatches fetch to the correct service based on view mode |

- Full-screen Bubble Tea app with alt screen
- **Service switcher** — press `[1]` for S3, `[2]` for SSM, `[3]` for SQS
- Dashboard sections: container status, then active service panel
- Docker client and `aws.Config` are created once in `initialModel` and cached in the `model` struct, reused across all TUI operations
- A cancellable `context.Context` is created at startup and cancelled on quit, ensuring in-flight operations are cancelled when the TUI exits
- Stale data remains visible during refresh — the view always shows the dashboard; a "⟳ refreshing..." indicator appears in the header instead of blanking the screen

**S3 mode:**
- Bucket list (default) → `enter` to browse objects
- Object view: `u` upload, `d` download, `del` delete (with confirmation), `esc` back

**SSM mode:**
- Parameter list, loaded one page at a time (20 items per page)
- `[` / `←` previous page, `]` / `→` next page
- `enter` fetches and displays the parameter value in the status line
- `del` / `backspace` deletes the selected parameter (with confirmation prompt)

**SQS mode:**
- Queue list → `enter` to browse messages
- `c` to create a new queue (with text input prompt)
- Message view: message body (truncated), `s` send message, `del` delete (with confirmation), `esc` back
- `enter` on a message shows its full body in the status line

**General keybindings:**
  - `↑`/`k` `↓`/`j` — navigate current list
  - `r` — refresh all data
  - `s` — start container (when not running) / send message (in SQS message view)
  - `x` — stop container (when running, requires `y` confirmation)
  - `q` / `ctrl+c` — quit
  - `esc` — go back / quit
- Destructive actions (delete object, delete parameter, delete queue, delete message, stop container) prompt for `y`/`N` confirmation before executing
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
| `internal/awsclient/` | 8 + shared err helpers | Config, credentials, retryer, 3 client factories, `IsConnectionErr`, `FriendlyErr` |
| `internal/s3ops/` | 28 | **100%** — S3 CRUD, error handling, pagination |
| `internal/ssmops/` | 16 | **100%** — SSM CRUD, pagination, error handling |
| `internal/sqsops/` | 28 | **100%** — SQS CRUD, pagination, error handling |
