import { Table } from "@cliffy/table";
import { blue, bold, dim, green, red, yellow } from "@std/fmt/colors";
import type { CostEstimate, DownloadProgress, UserConfig } from "../types.ts";

export function displayBanner(): void {
    console.log(bold(blue("\n🚀 RedStone Persister CLI\n")));
}

export function displayConfigSummary(config: UserConfig): void {
    const table = new Table()
        .header(["Configuration", "Value"])
        .body([
            ["Mode", config.mode],
            ["Exchanges", config.exchanges.join(", ")],
            ["Tokens", config.tokens.join(", ")],
            ["Start Date", config.startDate.toISOString().split("T")[0]],
            ["End Date", config.endDate.toISOString().split("T")[0]],
        ])
        .border(true);

    console.log(bold("\n📋 Configuration Summary:\n"));
    table.render();
    console.log("");
}

export function displayCostEstimate(estimate: CostEstimate): void {
    const table = new Table()
        .header(["Metric", "Value"])
        .body([
            ["Total Files", estimate.totalFiles.toString()],
            ["Total Size", `${estimate.totalSizeGB.toFixed(2)} GB`],
            [
                "Estimated Cost",
                bold(yellow(`$${estimate.estimatedCostUSD.toFixed(4)} USD`)),
            ],
        ])
        .border(true);

    console.log(bold("\n💰 Cost Estimate:\n"));
    table.render();

    if (estimate.filesByExchange.size > 1) {
        console.log(bold("\n📊 Breakdown by Exchange:\n"));
        const breakdownTable = new Table()
            .header(["Exchange", "Files", "Size (GB)"])
            .border(true);

        estimate.filesByExchange.forEach((count, exchange) => {
            const sizeGB = (estimate.sizeByExchange.get(exchange) || 0) / (1024 ** 3);
            breakdownTable.push([
                exchange.charAt(0).toUpperCase() + exchange.slice(1),
                count.toString(),
                sizeGB.toFixed(2),
            ]);
        });

        breakdownTable.render();
    }

    console.log("");
}

let lastProgressLineCount = 0;

export function displayDownloadProgress(progress: DownloadProgress): void {
    if (lastProgressLineCount > 0) {
        for (let i = 0; i < lastProgressLineCount; i++) {
            // Move cursor up and clear line
            Deno.stdout.writeSync(new TextEncoder().encode("\x1b[1A\x1b[2K"));
        }
    }

    const percentage = (progress.completedFiles / progress.totalFiles * 100).toFixed(1);
    const bar = createProgressBar(progress.completedFiles, progress.totalFiles);

    const lines: string[] = [];

    lines.push(bold(`Download Progress: ${percentage}%`));
    lines.push(`${bar} ${progress.completedFiles}/${progress.totalFiles} files`);

    if (progress.currentFiles.length > 0) {
        lines.push("");
        lines.push(dim("Currently downloading:"));
        progress.currentFiles.slice(0, 5).forEach((file) => {
            lines.push(dim(`  • ${file}`));
        });
        if (progress.currentFiles.length > 5) {
            lines.push(dim(`  ... and ${progress.currentFiles.length - 5} more`));
        }
    }

    if (progress.failedFiles > 0) {
        lines.push("");
        lines.push(red(`⚠ Failed: ${progress.failedFiles} files`));
    }

    console.log(lines.join("\n"));

    lastProgressLineCount = lines.length;
}

export function clearProgressDisplay(): void {
    if (lastProgressLineCount > 0) {
        for (let i = 0; i < lastProgressLineCount; i++) {
            Deno.stdout.writeSync(new TextEncoder().encode("\x1b[1A\x1b[2K"));
        }
        lastProgressLineCount = 0;
    }
}

function createProgressBar(current: number, total: number, width = 40): string {
    const percentage = current / total;
    const filled = Math.floor(percentage * width);
    const empty = width - filled;

    return green("█".repeat(filled)) + dim("░".repeat(empty));
}

export function displaySuccess(message: string): void {
    console.log(green("\n✓") + " " + message);
}

export function displayError(message: string): void {
    console.log(red("\n✗") + " " + message);
}

export function displayInfo(message: string): void {
    console.log(blue("\nℹ") + " " + message);
}

export function displayWarning(message: string): void {
    console.log(yellow("\n⚠") + " " + message);
}