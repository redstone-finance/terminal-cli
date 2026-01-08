# RedStone Terminal Data Downloader

A CLI tool to batch download historical cryptocurrency trade data (Parquet format) from RedStone Terminal.

## Installation

This tool is distributed as a standalone binary. You do not need to install Go or any dependencies.

1. **Locate the `bin` folder** in the project directory.
2. **Choose the binary** that matches your operating system:  

   | Operating System | Path | Description |
   | --- | --- | --- |
   | **macOS (Apple Silicon)** | `bin/darwin_arm64/terminal-cli` | For Apple Silicon Macs. |
   | **Linux** | `bin/linux_amd64/terminal-cli` | For standard Linux servers/desktops. |
   | **Windows** | `bin/windows_amd64/terminal-cli.exe` | For Windows 10/11. |

3. **Copy the binary** to your working directory (e.g., where you plan to run it).
4. *(Optional/Linux & Mac)* Ensure the file is executable:

```bash
chmod +x terminal-cli

```

## Configuration

You can provide an API Key via a `.env` file in the same directory as the binary, or pass it via flags.

**Create a `.env` file:**

```bash
API_KEY=your_secret_key_here

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
| `--api-key` |  | Manual API key entry (overrides `.env`) | No |  |
| `--yes` | `-y` | Skip confirmation prompts | No | `false` |
| `--help` | `-h` | Show help message | No |  |

> **Note:** The `--tokens` flag requires the full pair name (e.g., `btc_usdt`, `eth_usdc`). Passing just `btc` will not match any files.

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