export const INTERRUPT_CONFIRM_MS = 2_000;

export function createInterruptController(
    exit: (code?: number) => void,
    onPendingChange: (pending: boolean) => void,
) {
    let deadline = 0;
    let timer: Timer | null = null;

    const dispose = () => {
        if (timer !== null) {
            clearTimeout(timer);
            timer = null;
        }
        deadline = 0;
    };

    const cancel = () => {
        if (deadline !== 0) {
            dispose();
            onPendingChange(false);
        }
    };

    const request = () => {
        const now = Date.now();
        if (deadline > now) {
            cancel();
            exit(130);
            return;
        }
        cancel();
        deadline = now + INTERRUPT_CONFIRM_MS;
        onPendingChange(true);
        timer = setTimeout(cancel, INTERRUPT_CONFIRM_MS);
    };

    return { request, cancel, dispose };
}
