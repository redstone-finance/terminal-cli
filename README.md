# RedStone Terminal Data Downloader

A CLI tool to batch download historical cryptocurrency trade data (Parquet format) from RedStone Terminal.

## Installation

This tool is distributed as a standalone binary. You do not need to install Go or any dependencies.

1. **Download the binary** for your operating system from the [latest release](https://github.com/redstone-finance/terminal-cli/releases/latest):

   | Operating System | File |
   | --- | --- |
   | **macOS (Apple Silicon)** | `terminal-cli-darwin-arm64` |
   | **macOS (Intel)** | `terminal-cli-darwin-amd64` |
   | **Linux (x86-64)** | `terminal-cli-linux-amd64` |
   | **Linux (ARM64)** | `terminal-cli-linux-arm64` |
   | **Windows (x86-64)** | `terminal-cli-windows-amd64.exe` |
   | **Windows (ARM64)** | `terminal-cli-windows-arm64.exe` |

2. **Rename it** to `terminal-cli` (or `terminal-cli.exe` on Windows) and move it to your working directory.
3. *(Linux & Mac)* Make it executable:

```bash
chmod +x terminal-cli

```

`SHA256SUMS` in the release lists checksums for every binary.

To build all binaries from source into `bin/`, run `make build-all`.

### Publishing a release

Publish a release; the `Release` workflow then builds every platform and uploads the binaries and `SHA256SUMS` to it (about a minute):

```bash
gh release create v1.0.0 --generate-notes
```

Draft releases don't trigger the workflow until they are published.

## Configuration

You can provide an API Key via the `REDSTONE_TERMINAL_API_KEY` environment variable, a `.env` file in the directory you run the CLI from, or the `--api-key` flag.

**Export it:**

```bash
export REDSTONE_TERMINAL_API_KEY=your_secret_key_here
```

**Or create a `.env` file:**

```bash
REDSTONE_TERMINAL_API_KEY=your_secret_key_here

```

## Usage

```bash
# Mac/Linux
./terminal-cli [flags]

# Windows
.\terminal-cli.exe [flags]

```

### Modes

The CLI operates in two modes:

1. **`day` (Default)**: Downloads files. Requires `--exchanges` and `--tokens`.
2. **`check`**: Discovers available data. Displays a table of available tokens for the given date range.

### Options

| Flag | Shorthand | Description | Required | Default |
| --- | --- | --- | --- | --- |
| `--start-date` |  | Start date in `YYYY-MM-DD` format | **Yes** |  |
| `--end-date` |  | End date in `YYYY-MM-DD` format | No | Same as start |
| `--mode` |  | Operation mode: `day` or `check` | No | `day` |
| `--type` |  | Data type (`trade`, `derivative`) | No | `trade` |
| `--exchanges` |  | Comma-separated list of exchanges | **Yes** (for `day`) |  |
| `--tokens` |  | Comma-separated list of **full pairs** | **Yes** (for `day`) |  |
| `--parallel` | `-p` | Number of concurrent downloads | No | `10` |
| `--api-key` |  | API key (overrides `REDSTONE_TERMINAL_API_KEY`) | No |  |
| `--yes` | `-y` | Skip confirmation prompts | No | `false` |
| `--silent` | `-s` | Print nothing and skip prompts (implies `--yes`) | No | `false` |
| `--help` | `-h` | Show help message | No |  |

> **Note:** The `--tokens` flag requires the full pair name (e.g., `btc_usdt`, `eth_usdc`). Passing just `btc` will not match any files.

### Exit Status

| Code | Meaning |
| --- | --- |
| `0` | Every requested file is on disk (downloaded or already present). In `check` mode: data is available. |
| `1` | A download failed, nothing matched the criteria, or the prompt was declined. |
| `2` | Invalid flags or arguments, or a missing API key. |
| `3` | No download failed, but the server does not have some of the requested files yet. |

With `--silent` the exit status is the only signal, for example:

```bash
terminal-cli -s --type ticker --exchanges redstonelive --tokens btc_usd --start-date 2026-09-01 || echo "exit $?"
```

## Features

### 🚀 Parallel Downloading

By default, the CLI downloads **10 files** simultaneously. You can adjust this using the `--parallel` (or `-p`) flag.

* **`-p 1`**: Runs sequentially 
* **`-p >1`**: Runs concurrently

### ⏯️ Resume Capability

The tool automatically checks if a file already exists in the target directory **before** starting a download. If the file exists, it is marked as `Skipped` and the tool moves to the next job instantly, saving bandwidth and API quota.

### 🔍 Check Mode

Use `--mode check` to explore what data is available without downloading it.

* The output is grouped by time periods (if availability changes during the requested range).
* You can use `--exchanges` and `--tokens` in this mode to filter the results (e.g., "Is `btc_usdt` available on `binance`?").

## Output Directory

Downloaded files are saved in the `downloads/` folder, created in the same directory where you run the CLI.
The tool automatically organizes files by exchange, type, and date:

`./downloads/<exchange>/<type>/YYYY/MM/DD/<token_pair>/...`

## Examples

### 1. Download Data

**Basic single day download:**

```bash
./terminal-cli --exchanges binance --tokens btc_usdt --start-date 2025-11-02

```

**High-speed bulk download (20 parallel threads):**

```bash
./terminal-cli \
  --exchanges binance,bybit,gate \
  --tokens btc_usdt,eth_usdc,sol_usdt \
  --start-date 2025-11-01 \
  --end-date 2025-11-30 \
  --parallel 20

```

### 2. Check Availability

**See everything available for a date range:**

```bash
./terminal-cli --mode check --start-date 2025-11-01 --end-date 2025-11-05

```

**Check specific pairs on specific exchanges:**

```bash
./terminal-cli --mode check \
  --exchanges binance \
  --tokens btc_usdt,eth_usdt \
  --start-date 2025-11-01

```