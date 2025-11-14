/**
 * Type definitions for the Persister CLI
 */

export interface Manifest {
    defaultSource: string[];
    tokens: Record<string, TokenConfig>;
}

export interface TokenConfig {
    source: string[];
    isStableCoin?: boolean;
}

export type DataMode = "day" | "minute-parquet" | "minute-json";

export interface UserConfig {
    mode: DataMode;
    exchanges: string[];
    tokens: string[];
    startDate: Date;
    endDate: Date;
}

export interface S3FileInfo {
    bucket: string;
    key: string;
    exchange: string;
    token: string;
    date: Date;
    size?: number;
}

export interface CostEstimate {
    filesWithSize: S3FileInfo[];
    totalFiles: number;
    totalSizeBytes: number;
    totalSizeGB: number;
    estimatedCostUSD: number;
    filesByExchange: Map<string, number>;
    sizeByExchange: Map<string, number>;
}

export interface DownloadProgress {
    totalFiles: number;
    completedFiles: number;
    failedFiles: number;
    currentFiles: string[];
    totalBytes: number;
    downloadedBytes: number;
}

export interface DownloadResult {
    success: boolean;
    file: S3FileInfo;
    localPath?: string;
    error?: Error;
    downloadTimeMs: number;
}

export interface TokenAvailability {
    token: string;
    availableOn: Set<string>;
    isFullyAvailable: boolean;
    missingOn: string[];
}