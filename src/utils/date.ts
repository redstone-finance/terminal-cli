export function getAllDatesBetween(start: Date, end: Date): Date[] {
    const dates: Date[] = [];
    const current = new Date(start);

    while (current <= end) {
        dates.push(new Date(current));
        current.setDate(current.getDate() + 1);
    }

    return dates;
}

export function formatDate(date: Date): string {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");
    return `${year}-${month}-${day}`;
}

export function parseDate(dateStr: string): Date | null {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) {
        return null;
    }

    const date = new Date(dateStr);
    return isNaN(date.getTime()) ? null : date;
}

export function getDaysBetween(start: Date, end: Date): number {
    const msPerDay = 24 * 60 * 60 * 1000;
    return Math.floor((end.getTime() - start.getTime()) / msPerDay) + 1;
}