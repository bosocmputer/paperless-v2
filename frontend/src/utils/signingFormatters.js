const statusText = {
    draft: 'แบบร่าง',
    in_progress: 'รอเซ็น',
    pending_confirm: 'รอยืนยัน',
    pending: 'รอเซ็น',
    waiting: 'รอลำดับ',
    signed: 'เซ็นแล้ว',
    skipped: 'ข้ามแล้ว',
    rejected: 'ถูกปฏิเสธ',
    cancelled: 'ยกเลิก',
    completed: 'เสร็จสมบูรณ์',
    completed_evidence_failed: 'สร้าง PDF หลักฐานไม่สำเร็จ',
    completed_image_failed: 'ส่งรูป SML ไม่สำเร็จ',
    completed_lock_failed: 'Lock SML ไม่สำเร็จ'
};

const statusSeverity = {
    draft: 'secondary',
    in_progress: 'info',
    pending_confirm: 'warn',
    pending: 'info',
    waiting: 'secondary',
    signed: 'success',
    skipped: 'secondary',
    rejected: 'danger',
    cancelled: 'secondary',
    completed: 'success',
    completed_evidence_failed: 'warn',
    completed_image_failed: 'danger',
    completed_lock_failed: 'danger'
};

export function signingStatusLabel(status) {
    return statusText[status] || status || '-';
}

export function signingStatusSeverity(status) {
    return statusSeverity[status] || 'secondary';
}

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
        year: 'numeric'
    }).format(date);
}

export function formatThaiDateTime(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(date);
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
