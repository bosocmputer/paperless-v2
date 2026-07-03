import { formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';

const completedStatuses = new Set(['completed', 'signed']);
const waitingStatuses = new Set(['waiting', 'draft']);

export function buildWorkflowTimeline(document) {
    const steps = Array.isArray(document?.steps) ? document.steps : [];
    const signers = Array.isArray(document?.signers) ? document.signers : [];
    const groups = new Map();
    const naturalStepKeys = new Map();

    steps.forEach((step) => {
        const key = step.id || stepKey(step);
        groups.set(key, { step: normalizeStep(step, key), signers: [] });
        naturalStepKeys.set(stepKey(step), key);
    });

    signers.forEach((signer) => {
        const key = signer.stepId || naturalStepKeys.get(stepKey(signer)) || stepKey(signer);
        if (!groups.has(key)) groups.set(key, { step: normalizeStep(signer, key), signers: [] });
        groups.get(key).signers.push(signer);
    });

    return [...groups.values()]
        .sort((a, b) => Number(a.step.sequenceNo || 0) - Number(b.step.sequenceNo || 0) || String(a.step.positionCode || '').localeCompare(String(b.step.positionCode || '')))
        .map((group, index) => buildTimelineItem(group.step, group.signers, document, index));
}

export function documentProgressIssue(document) {
    const status = document?.status;
    if (status === 'completed_evidence_failed') {
        return {
            severity: 'warn',
            message: 'เอกสารเซ็นครบแล้ว แต่ยังสร้าง PDF หลักฐานไม่สำเร็จ ต้องสร้าง PDF อีกครั้งก่อนพิมพ์หรือส่งสถานะกลับ SML'
        };
    }
    if (status === 'completed_image_failed') {
        return {
            severity: 'error',
            message: 'เอกสารเซ็นครบและสร้าง PDF แล้ว แต่ยังส่งรูปเอกสารเข้า SML ไม่สำเร็จ ต้องส่งรูป SML อีกครั้งก่อน Lock SML หรือพิมพ์เอกสาร'
        };
    }
    if (status === 'completed_lock_failed') {
        return {
            severity: 'warn',
            message: 'เอกสารเซ็นครบและสร้าง PDF แล้ว แต่ยังส่งสถานะกลับ SML ไม่สำเร็จ ต้อง Lock SML อีกครั้ง'
        };
    }
    if (status === 'pending_confirm') {
        return {
            severity: 'info',
            message: 'เอกสารเซ็นครบแล้ว รอผู้ดูแลยืนยันเพื่อสร้าง PDF หลักฐานและส่งสถานะกลับ SML'
        };
    }
    if (status === 'rejected') {
        return {
            severity: 'error',
            message: 'เอกสารถูกปฏิเสธ workflow นี้หยุดแล้ว'
        };
    }
    return null;
}

export function conditionText(value) {
    if (Number(value) === 1) return 'ใครเซ็นก่อนก็ผ่าน';
    if (Number(value) === 2) return 'ต้องเซ็นครบทุกคน';
    if (Number(value) === 3) return 'รอผู้เซ็นภายนอก';
    return `เงื่อนไข ${value || '-'}`;
}

function buildTimelineItem(step, signers, document, index) {
    const sortedSigners = [...signers].sort((a, b) => Number(a.signerSlot || 0) - Number(b.signerSlot || 0) || String(a.signerUser || '').localeCompare(String(b.signerUser || '')));
    const signedCount = sortedSigners.filter((signer) => signer.status === 'signed').length;
    const totalCount = sortedSigners.length;
    const rejectedSigner = sortedSigners.find((signer) => signer.status === 'rejected');
    const status = resolveStepStatus(step, sortedSigners, document);
    const completedAt = step.completedAt || latestSignerDate(sortedSigners, 'signedAt') || rejectedSigner?.rejectedAt || '';
    const title = `${step.positionCode || index + 1} · ${step.positionName || 'ไม่ระบุขั้นตอน'}`;

    return {
        key: step.id || `${step.positionCode || index}-${step.sequenceNo || index}`,
        title,
        sequenceLabel: step.positionCode || `${index + 1}`,
        positionName: step.positionName || '',
        conditionType: Number(step.conditionType || 0),
        conditionLabel: conditionText(step.conditionType),
        summary: stepSummary(step, sortedSigners, signedCount, totalCount, status),
        status,
        statusLabel: progressStatusLabel(status),
        severity: progressSeverity(status),
        icon: progressIcon(status),
        muted: status === 'waiting' || status === 'skipped',
        completedAt,
        timeLabel: completedAt ? formatThaiDateTime(completedAt) : timelinePendingLabel(status),
        signers: sortedSigners.map(normalizeSigner)
    };
}

function normalizeStep(source, key) {
    return {
        id: source.id || source.stepId || key,
        positionCode: source.positionCode || '',
        positionName: source.positionName || '',
        sequenceNo: source.sequenceNo || 0,
        conditionType: source.conditionType || 0,
        status: source.status || 'waiting',
        completedAt: source.completedAt || ''
    };
}

function normalizeSigner(signer) {
    return {
        id: signer.id,
        signerType: signer.signerType,
        signerUser: signer.signerUser,
        label: signerLabel(signer),
        status: signer.status || 'waiting',
        statusLabel: signingStatusLabel(signer.status || 'waiting'),
        severity: signingStatusSeverity(signer.status || 'waiting'),
        signedAt: signer.signedAt || '',
        rejectedAt: signer.rejectedAt || ''
    };
}

function resolveStepStatus(step, signers, document) {
    if (step.status === 'rejected' || signers.some((signer) => signer.status === 'rejected')) return 'rejected';
    if (step.status === 'pending' || signers.some((signer) => signer.status === 'pending')) return 'pending';
    if (completedStatuses.has(step.status)) return 'completed';
    if (signers.length > 0 && signers.every((signer) => signer.status === 'skipped')) return 'skipped';
    if (signers.length > 0 && signers.every((signer) => signer.status === 'signed' || signer.status === 'skipped')) return 'completed';
    if ((document?.status === 'completed' || document?.status === 'pending_confirm') && signers.length === 0) return 'completed';
    if (waitingStatuses.has(step.status) || signers.some((signer) => signer.status === 'waiting')) return 'waiting';
    return step.status || 'waiting';
}

function stepSummary(step, signers, signedCount, totalCount, status) {
    if (status === 'rejected') return 'มีผู้ปฏิเสธเอกสาร ขั้นตอนนี้หยุดแล้ว';
    if (Number(step.conditionType) === 1) {
        const signed = signers.find((signer) => signer.status === 'signed');
        if (signed) return `${signerLabel(signed)} เซ็นแล้ว ขั้นตอนนี้ผ่าน`;
        return `มีผู้มีสิทธิ์เซ็น ${totalCount} คน, ใครเซ็นก่อนก็ผ่าน`;
    }
    if (Number(step.conditionType) === 2) return `เซ็นแล้ว ${signedCount}/${totalCount} คน`;
    if (Number(step.conditionType) === 3) return signedCount > 0 ? 'ผู้เซ็นภายนอกเซ็นแล้ว' : 'รอผู้เซ็นภายนอก';
    return totalCount ? `${totalCount} คนในขั้นตอนนี้` : 'ยังไม่มีผู้เซ็นในขั้นตอนนี้';
}

function progressStatusLabel(status) {
    const labels = {
        completed: 'เสร็จแล้ว',
        pending: 'กำลังรอเซ็น',
        waiting: 'ยังไม่ถึงคิว',
        skipped: 'ข้ามแล้ว',
        rejected: 'ถูกปฏิเสธ'
    };
    return labels[status] || signingStatusLabel(status);
}

function progressSeverity(status) {
    if (status === 'completed') return 'success';
    if (status === 'pending') return 'info';
    if (status === 'rejected') return 'danger';
    if (status === 'skipped') return 'secondary';
    return 'secondary';
}

function progressIcon(status) {
    if (status === 'completed') return 'pi pi-check';
    if (status === 'pending') return 'pi pi-clock';
    if (status === 'rejected') return 'pi pi-times';
    if (status === 'skipped') return 'pi pi-forward';
    return 'pi pi-circle';
}

function timelinePendingLabel(status) {
    if (status === 'pending') return 'รอดำเนินการ';
    if (status === 'waiting') return 'ยังไม่ถึงคิว';
    if (status === 'skipped') return 'ข้ามแล้ว';
    return '';
}

function latestSignerDate(signers, field) {
    return signers
        .map((signer) => signer[field])
        .filter(Boolean)
        .sort((a, b) => new Date(b).getTime() - new Date(a).getTime())[0];
}

function signerLabel(signer) {
    if (signer.signerType === 'external') return signer.signerName || 'บุคคลภายนอก';
    return signer.signerName || signer.signerUser || 'ไม่ระบุผู้เซ็น';
}

function stepKey(source) {
    return `${source.sequenceNo || 0}-${source.positionCode || source.positionName || 'step'}`;
}
