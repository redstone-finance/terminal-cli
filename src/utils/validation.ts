export function isValidDateFormat(dateStr: string): boolean {
    return /^\d{4}-\d{2}-\d{2}$/.test(dateStr);
}

export function isValidDate(date: Date): boolean {
    return !isNaN(date.getTime());
}

export function isValidDateRange(start: Date, end: Date): boolean {
    return isValidDate(start) && isValidDate(end) && start <= end;
}

export function isValidAWSRegion(region: string): boolean {
    const validRegions = [
        "us-east-1",
        "us-east-2",
        "us-west-1",
        "us-west-2",
        "eu-west-1",
        "eu-west-2",
        "eu-west-3",
        "eu-central-1",
        "ap-northeast-1",
        "ap-northeast-2",
        "ap-southeast-1",
        "ap-southeast-2",
        "ap-south-1",
        "sa-east-1",
    ];
    return validRegions.includes(region);
}