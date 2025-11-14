# Persister CLI

A command-line tool for fetching cryptocurrency trade data from RedStone S3 buckets.

## Installation

### Using Deno
```bash
deno install --allow-read --allow-net --allow-write --allow-env -n persister-cli https://deno.land/x/persister_cli/cli.ts
```

### From Source
```bash
git clone <repository-url>
cd persister-cli
deno task start
```

## Usage

### Interactive Mode
```bash
deno task start
```

The CLI will guide you through selecting mode, exchanges, tokens, and date range.

### Non-Interactive Mode (CLI Arguments)
```bash
# Download BTC data from Binance for a single day
deno task start --mode day --exchanges binance --tokens btc --start-date 2025-11-02

# Download multiple tokens from multiple exchanges
deno task start --mode day --exchanges binance,bybit,gate --tokens btc,eth,sol --start-date 2025-11-01 --end-date 2025-11-03

# Minute-level data in parquet format
deno task start --mode minute-parquet --exchanges binance --tokens btc,eth --start-date 2025-11-02

# Skip confirmation prompts with -y flag
deno task start --mode day --exchanges binance --tokens btc --start-date 2025-11-02 -y
```

### Command-Line Options
```
OPTIONS:
  --mode <mode>              Data mode: day, minute-parquet, minute-json
  --exchanges <exchanges>    Comma-separated list of exchanges (e.g., binance,bybit,gate)
  --tokens <tokens>          Comma-separated list of tokens (e.g., btc,eth,sol)
  --start-date <date>        Start date in YYYY-MM-DD format
  --end-date <date>          End date in YYYY-MM-DD format (defaults to start-date)
  --manifest <path>          Path to manifest file (default: ./examples/manifest.json)
  --yes, -y                  Skip confirmation prompts
  --help, -h                 Show help message
```

### Examples

**Single day download:**
```bash
deno task start --mode day --exchanges binance --tokens btc,eth --start-date 2025-11-02
```

**Date range with multiple exchanges:**
```bash
deno task start --mode day \
  --exchanges binance,bybit,gate \
  --tokens btc,eth,sol,link \
  --start-date 2025-11-01 \
  --end-date 2025-11-05
```

**Automated script (no confirmations):**
```bash
deno task start \
  --mode minute-parquet \
  --exchanges binance \
  --tokens btc \
  --start-date 2025-11-02 \
  --yes
```

## AWS Configuration

The CLI uses AWS SDK and requires AWS credentials. Configure them using:
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=eu-west-1
```

Or use AWS SSO:
```bash
aws sso login
```

Or use AWS credential files (`~/.aws/credentials`).

## Data Structure

### S3 Bucket Paths

**Daily files:**
```
s3://redstone-perun-persister-day/{exchange}/trade/{year}/{month}/{day}/{token}/{exchange}_trades_{date}_{token}.parquet
```

**Minute files (Parquet):**
```
s3://redstone-perun-persister-minute/parquet/{exchange}/trade/{year}/{month}/{day}/{token}/{exchange}_trades_{datetime}_{token}.parquet
```

**Minute files (JSON):**
```
s3://redstone-perun-persister-minute/json/{exchange}/trade/{year}/{month}/{day}/{token}/{exchange}_trades_{datetime}_{token}.json.gzip
```

## Cost Estimation

The CLI calculates data transfer costs based on:
- File sizes from S3 metadata
- AWS S3 data transfer pricing from eu-west-1
- Current rate: $0.09 per GB for the first 10 TB/month

## Development

### Running Tests
```bash
deno task test
```

### Type Checking
```bash
deno task check
```

### Linting
```bash
deno task lint
```

### Formatting
```bash
deno task fmt
```

## License

MIT