import type { Manifest, TokenAvailability } from "./types.ts";
import embeddedManifest from "../examples/manifest.json" with { type: "json" };


export async function loadManifest(path: string): Promise<Manifest> {
    if (!path) {
        return embeddedManifest as Manifest;
    }
    const content = await Deno.readTextFile(path);
    return JSON.parse(content);
}

export function extractExchanges(manifest: Manifest): string[] {
    const exchanges = new Set<string>();

    Object.values(manifest.tokens).forEach((token) => {
        token.source.forEach((source) => {
            // TODO: crypto-com...
            const match = source.match(/ws-trade-([^-]+)-/);
            if (match) {
                exchanges.add(match[1]);
            }
        });
    });

    return Array.from(exchanges).sort();
}

export function getTokenAvailability(
    manifest: Manifest,
    selectedExchanges: string[],
): TokenAvailability[] {
    const tokenAvailability: TokenAvailability[] = [];

    Object.entries(manifest.tokens).forEach(([token, config]) => {
        const availableOn = new Set<string>();

        config.source.forEach((source) => {
            const match = source.match(/ws-trade-([^-]+)-/);
            if (match && selectedExchanges.includes(match[1])) {
                availableOn.add(match[1]);
            }
        });

        if (availableOn.size > 0) {
            const isFullyAvailable = availableOn.size === selectedExchanges.length;
            const missingOn = selectedExchanges.filter((ex) => !availableOn.has(ex));

            tokenAvailability.push({
                token,
                availableOn,
                isFullyAvailable,
                missingOn,
            });
        }
    });

    return tokenAvailability.sort((a, b) => a.token.localeCompare(b.token));
}

export function validateTokenSelection(
    manifest: Manifest,
    tokens: string[],
    exchanges: string[],
): { valid: boolean; errors: string[] } {
    const errors: string[] = [];

    tokens.forEach((token) => {
        const config = manifest.tokens[token];
        if (!config) {
            errors.push(`Token ${token} not found in manifest`);
            return;
        }

        const availableExchanges = config.source
            .map((source) => source.match(/ws-trade-([^-]+)-/)?.[1])
            .filter((ex): ex is string => ex !== undefined);

        const missingExchanges = exchanges.filter((ex) => !availableExchanges.includes(ex));

        if (missingExchanges.length === exchanges.length) {
            errors.push(`Token ${token} is not available on any selected exchange`);
        }
    });

    return {
        valid: errors.length === 0,
        errors,
    };
}