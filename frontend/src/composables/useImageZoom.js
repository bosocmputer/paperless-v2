import { computed, onBeforeUnmount, ref } from 'vue';

const DEFAULT_MIN = 1;
const DEFAULT_MAX = 4;
const DEFAULT_STEP = 0.25;

// Shared by the SML gallery and the attachment preview: both show a scanned
// page fit to a dialog, which is too small to read an amount or a reference
// number, so both need to magnify and then move around the image.
export function useImageZoom({ min = DEFAULT_MIN, max = DEFAULT_MAX, step = DEFAULT_STEP } = {}) {
    const zoom = ref(min);
    const panX = ref(0);
    const panY = ref(0);
    const zoomed = computed(() => zoom.value > min);
    let panFrom = null;

    function clampZoom(value) {
        return Math.min(max, Math.max(min, Math.round(value * 100) / 100));
    }

    function resetZoom() {
        zoom.value = min;
        panX.value = 0;
        panY.value = 0;
    }

    function setZoom(value) {
        const next = clampZoom(value);
        if (next === min) {
            resetZoom();
            return;
        }
        zoom.value = next;
    }

    function zoomIn() {
        setZoom(zoom.value + step);
    }

    function zoomOut() {
        setZoom(zoom.value - step);
    }

    function onWheelZoom(event) {
        // Plain scrolling is left alone; only a deliberate ctrl/⌘ + wheel zooms,
        // the gesture browsers already use for zooming.
        if (!event.ctrlKey && !event.metaKey) return;
        event.preventDefault();
        setZoom(zoom.value + (event.deltaY < 0 ? step : -step));
    }

    function onPanMove(event) {
        if (!panFrom) return;
        panX.value = event.clientX - panFrom.x;
        panY.value = event.clientY - panFrom.y;
    }

    function onPanEnd() {
        panFrom = null;
        window.removeEventListener('pointermove', onPanMove);
    }

    function onPanStart(event) {
        if (!zoomed.value || event.button !== 0) return;
        event.preventDefault();
        panFrom = { x: event.clientX - panX.value, y: event.clientY - panY.value };
        window.addEventListener('pointermove', onPanMove);
        window.addEventListener('pointerup', onPanEnd, { once: true });
    }

    const imageTransform = computed(() => ({
        transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`
    }));

    // A drag that is still in progress when the component goes away would
    // otherwise leave its pointermove listener on window.
    onBeforeUnmount(onPanEnd);

    return { zoom, zoomed, min, max, step, setZoom, zoomIn, zoomOut, resetZoom, onWheelZoom, onPanStart, onPanEnd, imageTransform };
}
