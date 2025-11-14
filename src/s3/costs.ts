import {
    GetObjectCommand,
    HeadObjectCommand,
    S3Client,
} from "@aws-sdk/client-s3";
import type { CostEstimate, S3FileInfo } from "../types.ts";

// AWS S3 pricing for eu-west-1 (Ireland)
// Source: https://aws.amazon.com/s3/pricing/
const COST_PER_GB_TRANSFER = 0.09; // First 10 TB / month

export async function calculateCosts(
    files: S3FileInfo[],
    s3Client: S3Client,
): Promise<CostEstimate> {
    console.log(`\n🔍 Fetching metadata for ${files.length} files...`);

    // Fetch file sizes concurrently (in batches to avoid rate limits)
    const batchSize = 50;
    const filesWithSize: S3FileInfo[] = [];

    for (let i = 0; i < files.length; i += batchSize) {
        const batch = files.slice(i, i + batchSize);
        const results = await Promise.allSettled(
            batch.map((file) => getFileSize(file, s3Client)),
        );

        results.forEach((result, index) => {
            if (result.status === "fulfilled" && result.value !== null) {
                filesWithSize.push(result.value);
            }
        });

        // Progress indicator
        const progress = Math.min(i + batchSize, files.length);
        console.log(`  Processed ${progress}/${files.length} files...`);
    }

    // Calculate totals
    const totalSizeBytes = filesWithSize.reduce((sum, file) => sum + (file.size || 0), 0);
    const totalSizeGB = totalSizeBytes / (1024 ** 3);
    const estimatedCostUSD = totalSizeGB * COST_PER_GB_TRANSFER;

    // Group by exchange
    const filesByExchange = new Map<string, number>();
    const sizeByExchange = new Map<string, number>();

    filesWithSize.forEach((file) => {
        filesByExchange.set(file.exchange, (filesByExchange.get(file.exchange) || 0) + 1);
        sizeByExchange.set(
            file.exchange,
            (sizeByExchange.get(file.exchange) || 0) + (file.size || 0),
        );
    });

    return {
        filesWithSize,
        totalFiles: filesWithSize.length,
        totalSizeBytes,
        totalSizeGB,
        estimatedCostUSD,
        filesByExchange,
        sizeByExchange,
    };
}

async function getFileSize(
    file: S3FileInfo,
    s3Client: S3Client,
): Promise<S3FileInfo | null> {
    try {
        const command = new HeadObjectCommand({
            Bucket: file.bucket,
            Key: file.key,
        });

        const response = await s3Client.send(command);

        return {
            ...file,
            size: response.ContentLength || 0,
        };
    } catch (error) {
        return null;
    }
}