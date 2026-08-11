const statusText = {
    draft: 'แบบร่าง',
    in_progress: 'รอเซ็น',
    pending_confirm: 'รอส่งเข้า SML',
    auto_confirming: 'กำลังส่งเข้า SML',
    pending: 'รอเซ็น',
    waiting: 'รอลำดับ',
    signed: 'เซ็นแล้ว',
    skipped: 'ข้ามแล้ว',
    rejected: 'ถูกปฏิเสธ',
    cancelled: 'ยกเลิก',
    completed: 'เสร็จสมบูรณ์',
    completed_evidence_failed: 'สร้าง PDF หลักฐานไม่สำเร็จ',
    completed_image_failed: 'ส่งรูป SML ไม่สำเร็จ',
    completed_lock_failed: 'Lock SML ไม่สำเร็จ',
    sml_source_changed: 'ข้อมูล SML ถูกแก้ไข',
    sml_source_missing: 'ไม่พบเอกสารใน SML'
};

const statusSeverity = {
    draft: 'secondary',
    in_progress: 'info',
    pending_confirm: 'warn',
    auto_confirming: 'info',
    pending: 'info',
    waiting: 'secondary',
    signed: 'success',
    skipped: 'secondary',
    rejected: 'danger',
    cancelled: 'secondary',
    completed: 'success',
    completed_evidence_failed: 'warn',
    completed_image_failed: 'danger',
    completed_lock_failed: 'danger',
    sml_source_changed: 'warn',
    sml_source_missing: 'danger'
};

export function signingStatusLabel(status) {
    return statusText[status] || status || '-';
}

export function signingStatusSeverity(status) {
    return statusSeverity[status] || 'secondary';
}

// Both formatters below pin timeZone to Asia/Bangkok rather than letting
// Intl.DateTimeFormat fall back to the browser's local timezone. PaperLess
// stores timestamps as UTC internally, and SML's own timestamps are naive
// Asia/Bangkok values (see SML's *_datetime columns, confirmed timezone
// WITHOUT time zone) — pinning the display timezone here keeps what a user
// sees consistent with SML regardless of where their browser is set,
// instead of silently drifting for anyone outside Bangkok's UTC+7.
const THAI_TIME_ZONE = 'Asia/Bangkok';

export function formatDocumentDate(value) {
    if (!value) return '-';
    const text = String(value);
    const match = text.match(/^(\d{4})-(\d{2})-(\d{2})/);
    if (match) return `${match[3]}/${match[2]}/${match[1]}`;
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return new Intl.DateTimeFormat('th-TH-u-ca-gregory', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        timeZone: THAI_TIME_ZONE
    }).format(date);
}

export function formatThaiDateTime(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short', timeZone: THAI_TIME_ZONE }).format(date);
}

// Date-only, no time component, using Intl's 'medium' Thai date style
// (e.g. "31 ก.ค. 2569") — distinct from formatDocumentDate's DD/MM/YYYY
// numeric style. Callers that specifically need this style should import
// it from here rather than hand-rolling their own Intl.DateTimeFormat call.
export function formatThaiDate(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeZone: THAI_TIME_ZONE }).format(date);
}

// Numeric DD/MM/YYYY HH:mm, e.g. "31/07/2569 14:30" — distinct from
// formatThaiDateTime's word-based 'medium' style (e.g. "31 ก.ค. 2569
// 14:30"). Callers that specifically need the all-numeric layout should
// import this rather than hand-rolling their own Intl.DateTimeFormat call.
export function formatThaiDateTimeNumeric(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return new Intl.DateTimeFormat('th-TH', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        timeZone: THAI_TIME_ZONE
    }).format(date);
}

export function signingActionLabel(action) {
    const labels = {
        retry_final_pdf: 'สร้าง PDF อีกครั้ง',
        retry_sml_images: 'ส่งรูป SML อีกครั้ง',
        retry_sml_lock: 'Lock SML อีกครั้ง',
        fit_width: 'พอดีกว้าง',
        movement_log: 'เหตุการณ์สำคัญ',
        signature_preset: 'กรอบเริ่มต้น'
    };
    return labels[action] || action || '-';
}

export function smlImageFailureDetail(result = {}, fallback = 'ส่งรูปเอกสารเข้า SML ไม่สำเร็จ กรุณา retry') {
    const image = result.image || {};
    if (image.errorCode === 'tenant_image_database_missing') {
        const imageDatabase = image.errorDetails?.imageDatabase;
        return imageDatabase ? `ฐานข้อมูลรูป SML ยังไม่พร้อม: ${imageDatabase} กรุณาแจ้งผู้ดูแลระบบ แล้วกดส่งรูป SML อีกครั้งหลังแก้ไข` : 'ฐานข้อมูลรูป SML ยังไม่พร้อม กรุณาแจ้งผู้ดูแลระบบ แล้วกดส่งรูป SML อีกครั้งหลังแก้ไข';
    }
    return fallback;
}
