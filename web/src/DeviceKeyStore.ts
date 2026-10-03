const DB_NAME = "quartz-device-keys";
const STORE_NAME = "device-store";
const RECORD_ID = "active-device";

export interface StoredDeviceKeys {
    id: string;
    devicePrivateKey: CryptoKey;
    createdAt: number;
}

function openDB(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open(DB_NAME, 1);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains(STORE_NAME)) {
                db.createObjectStore(STORE_NAME, { keyPath: "id" });
            }
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

// Changed parameter type to CryptoKey
export async function saveDevicePrivateKey(
    devicePrivateKey: CryptoKey
): Promise<void> {
    const db = await openDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readwrite");
        const store = tx.objectStore(STORE_NAME);
        store.put({
            id: RECORD_ID,
            devicePrivateKey, // The CryptoKey object is natively supported by IndexedDB!
            createdAt: Date.now(),
        });
        tx.oncomplete = () => {
            db.close();
            resolve();
        };
        tx.onerror = () => {
            db.close();
            reject(tx.error);
        };
    });
}

// Changed return type to CryptoKey
export async function loadDevicePrivateKey(): Promise<CryptoKey | null> {
    const db = await openDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readonly");
        const store = tx.objectStore(STORE_NAME);
        const req = store.get(RECORD_ID);
        req.onsuccess = () => {
            db.close();
            if (req.result) {
                resolve(req.result.devicePrivateKey);
            } else {
                resolve(null);
            }
        };
        req.onerror = () => {
            db.close();
            reject(req.error);
        };
    });
}

export async function deleteDeviceKeys(): Promise<void> {
    return new Promise((resolve) => {
        const req = indexedDB.deleteDatabase(DB_NAME);
        req.onsuccess = () => resolve();
        req.onerror = () => resolve(); // Ignore errors on deletion
        req.onblocked = () => resolve();
    });
}
