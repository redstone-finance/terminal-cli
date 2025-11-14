import { GetObjectCommand, S3Client } from "@aws-sdk/client-s3";
import { ensureDir } from "@std/fs";
import { dirname } from "@std/path";
import type { DownloadProgress, DownloadResult, S3FileInfo } from "../types.ts";
import { getLocalPath } from "./paths.ts";

export async function downloadFiles(
    files: S3FileInfo[],
    s3Client: S3Client,
    concurrency = 10,
    onProgress?: (progress: DownloadProgress) => void,
): Promise<DownloadResult[]> {
    const results: DownloadResult[] = new Array(files.length);
    const progress: DownloadProgress = {
        totalFiles: files.length,
        completedFiles: 0,
        failedFiles: 0,
        currentFiles: [],
        totalBytes: files.reduce((sum, f) => sum + (f.size || 0), 0),
        downloadedBytes: 0,
    };

    onProgress?.(progress);

    const queue = files.map((_, i) => i);

    const worker = async () => {
        while (queue.length > 0) {
            const index = queue.shift();
            if (index === undefined) break;

            const file = files[index];
            const fileName = `${file.exchange}/${file.token}/${file.key.split("/").pop()}`;

            progress.currentFiles.push(fileName);
            onProgress?.(progress);

            const result = await downloadFile(file, s3Client);
            results[index] = result;

            if (result.success) {
                progress.completedFiles++;
                progress.downloadedBytes += file.size || 0;
            } else {
                progress.failedFiles++;
            }

            progress.currentFiles = progress.currentFiles.filter((f) => f !== fileName);
            onProgress?.(progress);
        }
    };

    const workers = Array.from({ length: Math.min(concurrency, files.length) }, () => worker());
    await Promise.all(workers);

    progress.currentFiles = [];
    onProgress?.(progress);

    return results;
}

async function downloadFile(
    file: S3FileInfo,
    s3Client: S3Client,
): Promise<DownloadResult> {
    const startTime = Date.now();

    try {
        const command = new GetObjectCommand({
            Bucket: file.bucket,
            Key: file.key,
        });

        const response = await s3Client.send(command);

        if (!response.Body) {
            throw new Error("Empty response body");
        }

        const bytes = await response.Body.transformToByteArray();

        const localPath = getLocalPath(file);
        await ensureDir(dirname(localPath));
        await Deno.writeFile(localPath, bytes);

        return {
            success: true,
            file,
            localPath,
            downloadTimeMs: Date.now() - startTime,
        };
    } catch (error) {
        return {
            success: false,
            file,
            error: error instanceof Error ? error : new Error(String(error)),
            downloadTimeMs: Date.now() - startTime,
        };
    }
}