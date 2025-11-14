#!/usr/bin/env -S deno run --allow-all

import { S3Client } from "@aws-sdk/client-s3";
import { red } from "@std/fmt/colors";
import { parseArgs } from "@std/cli/parse-args";
import { loadManifest, validateTokenSelection } from "./src/manifest.ts";
import {
    confirmConfig,
    confirmCosts,
    promptDateRange,
    promptExchanges,
    promptMode,
    promptTokens,
} from "./src/ui/prompts.ts";
import {
    displayBanner,
    displayConfigSummary,
    displayCostEstimate,
    displayDownloadProgress,
    displayError,
    displayInfo,
    displaySuccess,
    displayWarning,
    clearProgressDisplay,
} from "./src/ui/display.ts";
import { generateS3Paths } from "./src/s3/paths.ts";
import { calculateCosts } from "./src/s3/costs.ts";
import { downloadFiles } from "./src/s3/downloader.ts";
import type { DataMode, UserConfig } from "./src/types.ts";

interface CliArgs {
    mode?: string;
    exchanges?: string;
    tokens?: string;
    startDate?: string;
    endDate?: string;
    manifest?: string;
    help?: boolean;
    yes?: boolean;
}

function printHelp() {
    console.log(`
RedStone Persister CLI - Download cryptocurrency trade data from S3

USAGE:
  deno task start [OPTIONS]

OPTIONS:
  --mode <mode>              Data mode: day, minute-parquet, minute-json
  --exchanges <exchanges>    Comma-separated list of exchanges (e.g., binance,bybit,gate)
  --tokens <tokens>          Comma-separated list of tokens (e.g., btc,eth,sol)
  --start-date <date>        Start date in YYYY-MM-DD format
  --end-date <date>          End date in YYYY-MM-DD format (defaults to start-date)
  --manifest <path>          Path to manifest file (default: ./examples/manifest.json)
  --yes, -y                  Skip confirmation prompts
  --help, -h                 Show this help message

EXAMPLES:
  # Interactive mode
  deno task start

  # Download BTC and ETH data from Binance for a single day
  deno task start --mode day --exchanges binance --tokens btc,eth --start-date 2025-11-02

  # Download minute data from multiple exchanges for a date range
  deno task start --mode minute-parquet --exchanges binance,bybit,gate --tokens btc,eth --start-date 2025-11-01 --end-date 2025-11-03

  # Skip confirmation prompts
  deno task start --mode day --exchanges binance --tokens btc --start-date 2025-11-02 -y
`);
}

function parseCliArgs(args: string[]): CliArgs {
    const parsed = parseArgs(args, {
        string: ["mode", "exchanges", "tokens", "start-date", "end-date", "manifest"],
        boolean: ["help", "yes"],
        alias: {
            h: "help",
            y: "yes",
        },
    });

    return {
        mode: parsed.mode as string | undefined,
        exchanges: parsed.exchanges as string | undefined,
        tokens: parsed.tokens as string | undefined,
        startDate: parsed["start-date"] as string | undefined,
        endDate: parsed["end-date"] as string | undefined,
        manifest: parsed.manifest as string | undefined,
        help: parsed.help as boolean | undefined,
        yes: parsed.yes as boolean | undefined,
    };
}

function validateMode(mode: string): DataMode | null {
    const validModes = ["day", "minute-parquet", "minute-json"];
    if (validModes.includes(mode)) {
        return mode as DataMode;
    }
    return null;
}

function validateDate(dateStr: string): Date | null {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) {
        return null;
    }
    const date = new Date(dateStr);
    return isNaN(date.getTime()) ? null : date;
}

async function main() {
    const cliArgs = parseCliArgs(Deno.args);

    if (cliArgs.help) {
        printHelp();
        Deno.exit(0);
    }

    displayBanner();

    const manifestPath = cliArgs.manifest || "";
    let manifest;

    try {
        manifest = await loadManifest(manifestPath);
        displaySuccess("Manifest loaded successfully");
    } catch (error) {
        displayError(`Failed to load manifest: ${(error as Error).message}`);
        Deno.exit(1);
    }

    let config: UserConfig;

    const hasCliArgs = cliArgs.mode && cliArgs.exchanges && cliArgs.tokens && cliArgs.startDate;

    if (hasCliArgs) {
        // Non-interactive mode - parse CLI arguments
        displayInfo("Running in non-interactive mode with CLI arguments");

        const mode = validateMode(cliArgs.mode!);
        if (!mode) {
            displayError(
                `Invalid mode: ${cliArgs.mode}. Must be one of: day, minute-parquet, minute-json`,
            );
            Deno.exit(1);
        }

        const exchanges = cliArgs.exchanges!.split(",").map((e) => e.trim().toLowerCase()).filter((
            e,
        ) => e.length > 0);
        if (exchanges.length === 0) {
            displayError("No valid exchanges provided");
            Deno.exit(1);
        }

        const tokens = cliArgs.tokens!.split(",").map((t) => t.trim().toUpperCase()).filter((t) =>
            t.length > 0
        );
        if (tokens.length === 0) {
            displayError("No valid tokens provided");
            Deno.exit(1);
        }

        const validation = validateTokenSelection(manifest, tokens, exchanges);
        if (!validation.valid) {
            displayWarning("Some token/exchange combinations are not available:");
            validation.errors.forEach((error) => {
                console.log(`  - ${error}`);
            });
            displayInfo("Proceeding with available combinations only...");
        }

        const startDate = validateDate(cliArgs.startDate!);
        if (!startDate) {
            displayError(`Invalid start date: ${cliArgs.startDate}. Use YYYY-MM-DD format`);
            Deno.exit(1);
        }

        const endDate = cliArgs.endDate ? validateDate(cliArgs.endDate) : startDate;
        if (!endDate) {
            displayError(`Invalid end date: ${cliArgs.endDate}. Use YYYY-MM-DD format`);
            Deno.exit(1);
        }

        if (endDate < startDate) {
            displayError("End date must be after or equal to start date");
            Deno.exit(1);
        }

        config = {
            mode,
            exchanges,
            tokens,
            startDate,
            endDate,
        };

        displayConfigSummary(config);

        if (!cliArgs.yes) {
            const proceed = await confirmConfig(config);
            if (!proceed) {
                displayInfo("Operation cancelled.");
                Deno.exit(0);
            }
        }
    } else {
        // Interactive mode - use prompts
        if (
            cliArgs.mode || cliArgs.exchanges || cliArgs.tokens || cliArgs.startDate || cliArgs.endDate
        ) {
            displayWarning(
                "Partial CLI arguments detected. All of --mode, --exchanges, --tokens, and --start-date are required for non-interactive mode.",
            );
            displayInfo("Falling back to interactive mode...\n");
        }

        const mode = await promptMode();
        console.log("");

        const exchanges = await promptExchanges(manifest);
        console.log("");

        const tokens = await promptTokens(manifest, exchanges);
        console.log("");

        const { startDate, endDate } = await promptDateRange(mode);
        console.log("");

        config = {
            mode,
            exchanges,
            tokens,
            startDate,
            endDate,
        };

        displayConfigSummary(config);

        const proceed = await confirmConfig(config);
        if (!proceed) {
            displayInfo("Operation cancelled.");
            Deno.exit(0);
        }
    }

    displaySuccess("Configuration accepted!");

    const s3Client = new S3Client({
        region: Deno.env.get("AWS_REGION") || "eu-west-1",
    });

    displayInfo("Generating file paths...");
    const files = generateS3Paths(config);
    displaySuccess(`Generated ${files.length} file paths`);

    let filesWithSize = []
    try {
        const costEstimate = await calculateCosts(files, s3Client);
        filesWithSize = costEstimate.filesWithSize;
        displayCostEstimate(costEstimate);


        if (costEstimate.totalFiles === 0) {
            displayError("No files found for the selected configuration.");
            Deno.exit(1);
        }

        const confirmDownload = cliArgs.yes
            ? true
            : await confirmCosts(costEstimate.estimatedCostUSD);
        if (!confirmDownload) {
            displayInfo("Download cancelled.");
            Deno.exit(0);
        }
    } catch (error) {
        displayError(`Failed to calculate costs: ${(error as Error).message}`);
        Deno.exit(1);
    }

    displayInfo("Starting download...\n");

    try {
        const results = await downloadFiles(
            filesWithSize.filter((f) => f.size !== undefined && f.size > 0),
            s3Client,
            10,
            displayDownloadProgress,
        );

        clearProgressDisplay();

        const successful = results.filter((r) => r.success).length;
        const failed = results.filter((r) => !r.success).length;

        displaySuccess(`Download complete! ${successful} files downloaded`);

        if (failed > 0) {
            displayError(`${failed} files failed to download`);

            const failedResults = results.filter((r) => !r.success).slice(0, 5);
            console.log("\nFailed files:");
            failedResults.forEach((r) => {
                console.log(`  - ${r.file.key}: ${r.error?.message}`);
            });
            if (failed > 5) {
                console.log(`  ... and ${failed - 5} more`);
            }
        }
    } catch (error) {
        displayError(`Download failed: ${(error as Error).message}`);
        Deno.exit(1);
    }
}

if (import.meta.main) {
    main().catch((error) => {
        console.error(red("\n✗ Fatal error:"), error);
        Deno.exit(1);
    });
}