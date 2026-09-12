let busy = false;
export function acquireWalletOperation() {
    if (busy) throw Error('Another wallet operation is in progress. Wait for it to finish.');
    busy = true;
    return () => { busy = false; };
}
