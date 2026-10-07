const DAY_MS = 24 * 60 * 60 * 1000;

// How many days of trial are left, and whether it has already ended.
//
// "Ended" is decided from the remaining time itself (<= 0), never from the day
// count. The day count is rounded up, so it stays at 0 for the whole first 24
// hours after the deadline - which made an already-ended trial read as "ends
// today" in amber, while the server was refusing logins, and only flipped to
// "ended" a day late.
export function trialBannerState(expiresAt, now = Date.now()) {
    if (!expiresAt) return { visible: false, expired: false, daysLeft: null };
    const expiresMs = new Date(expiresAt).getTime();
    if (Number.isNaN(expiresMs)) return { visible: false, expired: false, daysLeft: null };

    const remainingMs = expiresMs - now;
    if (remainingMs <= 0) return { visible: true, expired: true, daysLeft: 0 };

    const daysLeft = Math.ceil(remainingMs / DAY_MS);
    return { visible: daysLeft <= 3, expired: false, daysLeft };
}

export function trialBannerMessage({ expired, daysLeft }) {
    if (expired) return 'ระยะเวลาทดลองใช้งานสิ้นสุดแล้ว กรุณาติดต่อทีมงานเพื่อต่ออายุการใช้งาน';
    return `ระยะเวลาทดลองใช้งานจะสิ้นสุดในอีก ${daysLeft} วัน กรุณาติดต่อทีมงานเพื่อต่ออายุการใช้งาน`;
}
