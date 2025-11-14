import type { DataMode, S3FileInfo, UserConfig } from "../types.ts";
import { getAllDatesBetween } from "../utils/date.ts";

export function generateS3Paths(config: UserConfig): S3FileInfo[] {
    const files: S3FileInfo[] = [];
    const dates = getAllDatesBetween(config.startDate, config.endDate);

    for (const exchange of config.exchanges) {
        for (const token of config.tokens) {
            for (const date of dates) {
                if (config.mode === "day") {
                    files.push(generateDailyPath(exchange, token, date));
                } else {
                    files.push(...generateMinutePaths(exchange, token, date, config.mode));
                }
            }
        }
    }

    return files;
}

function generateDailyPath(exchange: string, token: string, date: Date): S3FileInfo {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");
    const dateStr = `${year}-${month}-${day}`;

    const key =
        `${exchange}/trade/${year}/${month}/${day}/${token.toLowerCase()}/${exchange}_trades_${dateStr}_${token.toLowerCase()}.parquet`;

    return {
        bucket: "redstone-perun-persister-day",
        key,
        exchange,
        token,
        date,
    };
}

function generateMinutePaths(
    exchange: string,
    token: string,
    date: Date,
    mode: DataMode,
): S3FileInfo[] {
    const files: S3FileInfo[] = [];
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");

    const isParquet = mode === "minute-parquet";
    const bucket = "redstone-perun-persister-minute";
    const format = isParquet ? "parquet" : "json";
    const extension = isParquet ? "parquet" : "json.gzip";

    for (let hour = 0; hour < 24; hour++) {
        for (let minute = 0; minute < 60; minute++) {
            const hourStr = String(hour).padStart(2, "0");
            const minuteStr = String(minute).padStart(2, "0");
            const datetime = `${year}-${month}-${day}T${hourStr}-${minuteStr}-00`;

            const key =
                `${format}/${exchange}/trade/${year}/${month}/${day}/${token.toLowerCase()}/${exchange}_trades_${datetime}_${token.toLowerCase()}.${extension}`;

            files.push({
                bucket,
                key,
                exchange,
                token,
                date: new Date(year, date.getMonth(), date.getDate(), hour, minute),
            });
        }
    }

    return files;
}

export function getLocalPath(file: S3FileInfo, baseDir = "./downloads"): string {
    const date = file.date;
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");

    return `${baseDir}/${file.exchange}/${year}/${month}/${day}/${file.token.toLowerCase()}/${
        file.key.split("/").pop()
    }`;
}