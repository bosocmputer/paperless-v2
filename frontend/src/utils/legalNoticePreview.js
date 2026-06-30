export const LEGAL_NOTICE_TEXT =
    'เอกสารนี้จัดทำและลงนามในรูปแบบอิเล็กทรอนิกส์ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ พ.ศ. 2544 ผู้ลงนามยืนยันความถูกต้องของเนื้อหาและยอมรับผลผูกพันทางกฎหมายทุกประการ';

export const LEGAL_NOTICE_DISPLAY_TEXT =
    'เอกสารนี้จัดทำและลงนามในรูปแบบอิเล็กทรอนิกส์ตาม พระราชบัญญัติธุรกรรมทางอิเล็กทรอนิกส์ พุทธศักราช ๒๕๔๔ ผู้ลงนามยืนยันความถูกต้องของเนื้อหาและยอมรับผลผูกพันทางกฎหมายทุกประการ';

export function legalNoticePreviewFontSize(zoom = 1) {
    const scale = Number(zoom) || 1;
    return Math.max(7, Math.min(13, 9.2 * scale));
}

export function legalNoticeOverflowMessage() {
    return 'ข้อความกฎหมายอาจล้นกรอบ กรุณาขยายกรอบหรือเพิ่มความสูง';
}
