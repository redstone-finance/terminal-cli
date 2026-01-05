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

### Options

| Flag           | Shorthand | Description                                               | Required | Default       |
|----------------| --- |-----------------------------------------------------------| --- |---------------|
| `--exchanges`  |  | Comma-separated list of exchanges (e.g., `binance,bybit`) | **Yes** |               |
| `--tokens`     |  | Comma-separated list of **full pairs** (e.g., `btc_usdt`) | **Yes** |               |
| `--start-date` |  | Start date in `YYYY-MM-DD` format                         | **Yes** |               |
| `--end-date`   |  | End date in `YYYY-MM-DD` format                           | No | Same as start |
| `--mode`       |  | Data mode                                                 | No | `day`         |
| `--type`       |  | Data type (`trade`, `derivative`)                         | No | `trade`       |
| `--api-key`    |  | Manual API key entry (overrides `.env`)                   | No |               |
| `--yes`        | `-y` | Skip confirmation prompts                                 | No | `false`       |
| `--help`       | `-h` | Show help message                                         | No |               |

> **Note:** The `--tokens` flag requires the full pair name (e.g., `btc_usdt`, `eth_usdc`). Passing just `btc` will not match any files.

### Output Directory

Downloaded files are saved in the `downloads/` folder, created in the same directory where you run the CLI.
The tool automatically organizes files by exchange and date:

`./downloads/<exchange>/trade/YYYY/MM/DD/<token_pair>/...`

## Examples

**1. Single day download:**

```bash
./terminal-cli --exchanges binance --tokens btc_usdt --start-date 2025-11-02

```

**2. Multiple exchanges and tokens over a date range:**

```bash
./terminal-cli \
  --exchanges binance,bybit,gate \
  --tokens btc_usdt,eth_usdc,sol_usdt \
  --start-date 2025-11-01 \
  --end-date 2025-11-03

```

**3. Automated execution (skip confirmation):**

```bash
./terminal-cli --exchanges okx --tokens ton_usdt --start-date 2025-03-21 -y

```