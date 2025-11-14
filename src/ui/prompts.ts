import { Checkbox, Confirm, Input, Select } from "@cliffy/prompt";
import { bold, dim, green, yellow } from "@std/fmt/colors";
import type { DataMode, Manifest, UserConfig } from "../types.ts";
import { extractExchanges, getTokenAvailability } from "../manifest.ts";

export async function promptMode(): Promise<DataMode> {
    const mode = await Select.prompt({
        message: "Select data mode",
        options: [
            { name: "Daily aggregated data (Parquet)", value: "day" },
            { name: "Minute-level data (Parquet)", value: "minute-parquet" },
            { name: "Minute-level data (JSON gzipped)", value: "minute-json" },
        ],
    });
    return mode as DataMode;
}

export async function promptExchanges(manifest: Manifest): Promise<string[]> {
    const allExchanges = extractExchanges(manifest);

    const exchanges = await Checkbox.prompt({
        message: "Select exchanges (Space to select, Enter to confirm)",
        options: allExchanges.map((ex) => ({
            name: ex.charAt(0).toUpperCase() + ex.slice(1),
            value: ex,
        })),
        minOptions: 1,
    });

    return exchanges;
}

export async function promptTokens(
    manifest: Manifest,
    selectedExchanges: string[],
): Promise<string[]> {
    const tokenAvailability = getTokenAvailability(manifest, selectedExchanges);

    console.log("\n" + bold("Token availability legend:"));
    console.log(green("✓") + " Available on all selected exchanges");
    console.log(yellow("⚠") + " Available on some selected exchanges");
    console.log("");

    const options = tokenAvailability.map((info) => {
        const indicator = info.isFullyAvailable
            ? green("✓")
            : yellow("⚠");

        const suffix = info.isFullyAvailable
            ? ""
            : dim(` (missing: ${info.missingOn.join(", ")})`);

        return {
            name: `${indicator} ${info.token}${suffix}`,
            value: info.token,
        };
    });

    const tokens = await Checkbox.prompt({
        message: "Select tokens (Space to select, Enter to confirm)",
        options,
        minOptions: 1,
        hint: "Note: Tokens marked with ⚠ won't be fetched from all selected exchanges",
    });

    return tokens;
}

export async function promptDateRange(
    mode: DataMode,
): Promise<{ startDate: Date; endDate: Date }> {
    const hint = mode === "day"
        ? "Format: YYYY-MM-DD (e.g., 2025-01-15)"
        : "Format: YYYY-MM-DD (minute data will be fetched for full days)";

    const startDateStr = await Input.prompt({
        message: "Enter start date",
        hint,
        validate: (value) => {
            if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
                return "Invalid format. Use YYYY-MM-DD";
            }
            const date = new Date(value);
            if (isNaN(date.getTime())) {
                return "Invalid date";
            }
            return true;
        },
    });

    const endDateStr = await Input.prompt({
        message: "Enter end date",
        hint: hint + dim(" (press Enter to use same as start date)"),
        default: startDateStr,
        validate: (value) => {
            if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
                return "Invalid format. Use YYYY-MM-DD";
            }
            const date = new Date(value);
            if (isNaN(date.getTime())) {
                return "Invalid date";
            }
            const startDate = new Date(startDateStr);
            if (date < startDate) {
                return "End date must be after start date";
            }
            return true;
        },
    });

    return {
        startDate: new Date(startDateStr),
        endDate: new Date(endDateStr),
    };
}

export async function confirmConfig(config: UserConfig): Promise<boolean> {
    return await Confirm.prompt({
        message: "Proceed with this configuration?",
        default: true,
    });
}

export async function confirmCosts(estimatedCostUSD: number): Promise<boolean> {
    return await Confirm.prompt({
        message: `Estimated cost: $${estimatedCostUSD.toFixed(4)}. Continue with download?`,
        default: true,
    });
}