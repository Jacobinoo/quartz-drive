const DB_NAME = "quartz-search-index";
const STORE_NAME = "files-store";

export interface DecryptedSearchItem {
    id: string;
    name: string;
    type: string;
    sizeBytes: number;
    createdAt: string;
}

function openSearchDB(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open(DB_NAME, 1);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains(STORE_NAME)) {
                // We use the file's ID as the primary key
                db.createObjectStore(STORE_NAME, { keyPath: "id" });
            }
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

// Saves an array of fully decrypted files into the browser DB
export async function saveSearchIndex(
    items: DecryptedSearchItem[]
): Promise<void> {
    const db = await openSearchDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readwrite");
        const store = tx.objectStore(STORE_NAME);

        // Clear the old index before saving the new one to prevent stale data
        store.clear();

        items.forEach((item) => store.put(item));

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

// Loads the entire search index into memory for hyper-fast querying
export async function loadSearchIndex(): Promise<DecryptedSearchItem[]> {
    const db = await openSearchDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readonly");
        const store = tx.objectStore(STORE_NAME);
        const request = store.getAll();

        request.onsuccess = () => {
            db.close();
            resolve(request.result || []);
        };
        request.onerror = () => {
            db.close();
            reject(request.error);
        };
    });
}

// Wipes the search index (call this when the user logs out!)
export async function clearSearchIndex(): Promise<void> {
    const db = await openSearchDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readwrite");
        const store = tx.objectStore(STORE_NAME);
        store.clear();
        tx.oncomplete = () => {
            db.close();
            resolve();
        };
    });
}

// Incrementally add a single new file or folder to the index without a full rebuild!
export async function addSingleSearchItem(
    item: DecryptedSearchItem
): Promise<void> {
    const db = await openSearchDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readwrite");
        const store = tx.objectStore(STORE_NAME);

        store.put(item); // .put will insert or update!

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

// Remove a file or folder from the local search index
export async function removeSearchItem(nodeId: string): Promise<void> {
    const db = await openSearchDB();
    return new Promise((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, "readwrite");
        const store = tx.objectStore(STORE_NAME);

        store.delete(nodeId); // Delete by the primary key (nodeId)

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
