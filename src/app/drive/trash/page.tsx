"use client";

import { useEffect, useState, useRef } from "react";
import { getSodium } from "@/lib/crypto/sodium";
import { useDriveStore, type FolderKey } from "@/lib/driveStore";

import { FileIcon, FolderIcon, RefreshCw, Trash, Loader2 } from "lucide-react";
import { customFetch } from "@/lib/api";
import { getAccountEncryptionPrivateKey } from "@/lib/authStore";
import { initializeDriveKeys } from "@/crypto/drive";

interface TrashedItem {
  nodeId: string;
  parentNodeId: string;
  type: string;
  encryptedName: string;
  nameNonce: string;
  encryptedNodePassphrase: string;
  signedEncryptedNodePassphrase: string;
  createdAt: string;
  // Populated after decryption:
  plaintextName?: string;
  locationPath?: string;
  decrypted?: boolean;
}

// Cache of already-resolved folder keys by nodeId
const keyCache = new Map<string, { privateKey: Uint8Array; publicKey: Uint8Array }>();

export default function TrashPage() {
  const [trashedFiles, setTrashedFiles] = useState<TrashedItem[]>([]);
  const [loading, setLoading] = useState(true);
  const myDriveRoot = useDriveStore((s) => s.myDriveRoot);
  const abortRef = useRef(false);

  useEffect(() => {
    abortRef.current = false;
    loadGlobalTrash();
    return () => { abortRef.current = true; };
  }, [myDriveRoot]);

  async function loadGlobalTrash() {
    setLoading(true);
    try {
      // Ensure root keys are initialized even if the user landed directly on the trash page
      await initializeDriveKeys();

      // Seed the cache from the (now guaranteed) root
      const root = useDriveStore.getState().myDriveRoot;
      if (root) {
        keyCache.set(root.nodeId, { privateKey: root.privateKey, publicKey: root.publicKey });
      }
      const res = await customFetch("https://localhost:3100/v1/files/trash/list");
      if (!res.ok) return;
      const rawFiles: TrashedItem[] | null = await res.json();
      if (!rawFiles || rawFiles.length === 0) {
        setTrashedFiles([]);
        setLoading(false);
        return;
      }

      // Show them immediately as "loading" placeholders
      setTrashedFiles(rawFiles.map((f) => ({ ...f, decrypted: false })));
      setLoading(false);

      // Progressively decrypt each item
      await decryptAllItems(rawFiles);
    } catch (err) {
      console.error("Failed to load global trash:", err);
      setLoading(false);
    }
  }

  async function decryptAllItems(items: TrashedItem[]) {
    const sodium = await getSodium();

    // Group items by parentNodeId to batch ancestor lookups
    const byParent = new Map<string, TrashedItem[]>();
    for (const item of items) {
      const list = byParent.get(item.parentNodeId) || [];
      list.push(item);
      byParent.set(item.parentNodeId, list);
    }

    for (const [parentNodeId, groupItems] of byParent) {
      if (abortRef.current) return;

      try {
        // Resolve the key for this parent folder
        const parentKey = await resolveKeyChain(sodium, parentNodeId);
        if (!parentKey) {
          // Mark these items as failed to decrypt
          for (const item of groupItems) {
            updateItem(item.nodeId, { plaintextName: "⚠ Decryption Failed", locationPath: "Unknown", decrypted: true });
          }
          continue;
        }

        // Now decrypt every item in this group
        for (const item of groupItems) {
          if (abortRef.current) return;
          try {
            const ciphertext = sodium.from_base64(item.encryptedName);
            const nonce = sodium.from_base64(item.nameNonce);
            const decryptedBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
              null, ciphertext, null, nonce, parentKey.privateKey
            );
            const plaintextName = sodium.to_string(decryptedBytes);

            // Resolve the location path (parent chain names)
            const locationPath = await resolveLocationPath(sodium, parentNodeId);

            updateItem(item.nodeId, { plaintextName, locationPath, decrypted: true });
          } catch (e) {
            console.error(`Failed to decrypt item ${item.nodeId}:`, e);
            updateItem(item.nodeId, { plaintextName: "⚠ Decryption Failed", locationPath: "Unknown", decrypted: true });
          }
        }
      } catch (e) {
        console.error(`Failed to resolve key chain for parent ${parentNodeId}:`, e);
        for (const item of groupItems) {
          updateItem(item.nodeId, { plaintextName: "⚠ Decryption Failed", locationPath: "Unknown", decrypted: true });
        }
      }
    }
  }

  function updateItem(nodeId: string, updates: Partial<TrashedItem>) {
    setTrashedFiles((prev) =>
      prev.map((f) => (f.nodeId === nodeId ? { ...f, ...updates } : f))
    );
  }

  /**
   * Resolves the key for a given nodeId by walking the ancestor chain.
   * Uses caching so repeated lookups are instant.
   */
  async function resolveKeyChain(
    sodium: any,
    nodeId: string
  ): Promise<{ privateKey: Uint8Array; publicKey: Uint8Array } | null> {
    // Check cache first
    if (keyCache.has(nodeId)) {
      return keyCache.get(nodeId)!;
    }

    // Fetch the ancestor chain from the backend
    const res = await customFetch(
      `https://localhost:3100/v1/files/path?nodeId=${nodeId}`
    );
    if (!res.ok) return null;
    const ancestors: any[] = await res.json();
    if (!ancestors || ancestors.length === 0) return null;

    // The chain is ordered root-first. We need to walk it from the root down.
    // The root's key should already be in the cache (seeded from myDriveRoot).
    // Each subsequent entry is a child whose passphrase is sealed with the parent's key.

    let currentKey = keyCache.get(ancestors[0].nodeId);
    if (!currentKey && myDriveRoot && ancestors[0].nodeId === myDriveRoot.nodeId) {
      currentKey = { privateKey: myDriveRoot.privateKey, publicKey: myDriveRoot.publicKey };
      keyCache.set(myDriveRoot.nodeId, currentKey);
    }

    if (!currentKey) {
      console.error("Root key not found in cache for chain walk");
      return null;
    }

    // Walk from index 1 (first child of root) down to the target
    for (let i = 1; i < ancestors.length; i++) {
      const entry = ancestors[i];
      const cachedKey = keyCache.get(entry.nodeId);
      if (cachedKey) {
        currentKey = cachedKey;
        continue;
      }

      try {
        // Unwrap passphrase using parent's keypair
        const nodePassphrase = sodium.crypto_box_seal_open(
          sodium.from_base64(entry.encryptedNodePassphrase),
          currentKey!.publicKey,
          currentKey!.privateKey
        );

        // Decrypt child's private key
        const childPrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
          null,
          sodium.from_base64(entry.wrappedNodeKey),
          sodium.from_string("FolderNode"),
          sodium.from_base64(entry.nodePrivNonce),
          nodePassphrase
        );

        const childPubKey = sodium.from_base64(entry.nodePublicKey);

        currentKey = { privateKey: childPrivKey, publicKey: childPubKey };
        keyCache.set(entry.nodeId, currentKey);
      } catch (e) {
        console.error(`Failed to unlock key for node ${entry.nodeId}:`, e);
        return null;
      }
    }

    return currentKey;
  }

  // Location path cache
  const locationCache = new Map<string, string>();

  async function resolveLocationPath(sodium: any, parentNodeId: string): Promise<string> {
    if (locationCache.has(parentNodeId)) {
      return locationCache.get(parentNodeId)!;
    }

    try {
      const res = await customFetch(
        `https://localhost:3100/v1/files/path?nodeId=${parentNodeId}`
      );
      if (!res.ok) return "Unknown";
      const ancestors: any[] = await res.json();
      if (!ancestors || ancestors.length === 0) return "Unknown";

      // Build the plaintext path by decrypting each ancestor's name
      const parts: string[] = ["My Drive"];

      for (let i = 1; i < ancestors.length; i++) {
        const entry = ancestors[i];
        const parentKey = keyCache.get(ancestors[i - 1].nodeId);
        if (!parentKey) {
          parts.push("?");
          continue;
        }
        try {
          const decryptedBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(entry.encryptedName),
            null,
            sodium.from_base64(entry.nameNonce),
            parentKey.privateKey
          );
          parts.push(sodium.to_string(decryptedBytes));
        } catch {
          parts.push("?");
        }
      }

      const path = parts.join(" / ");
      locationCache.set(parentNodeId, path);
      return path;
    } catch {
      return "Unknown";
    }
  }

  const handleRestore = async (file: TrashedItem) => {
    try {
      const res = await customFetch(
        `https://localhost:3100/v1/files/restore?nodeId=${file.nodeId}&parentFolderId=${file.parentNodeId}`,
        { method: "POST" }
      );
      if (res.ok) {
        // The backend may have also restored trashed ancestor folders.
        // Reload the global trash list to get the accurate new state.
        await loadGlobalTrash();
      }
    } catch (e) {
      console.error("Restore failed:", e);
    }
  };

  const handleEmptyTrash = async () => {
    try {
      const res = await customFetch(
        `https://localhost:3100/v1/files/trash/empty`,
        { method: "DELETE" }
      );
      if (res.ok) {
        setTrashedFiles([]);
        alert("Trash emptied! S3 chunks are being wiped asynchronously in the background.");
      }
    } catch (e) {
      console.error("Empty trash failed:", e);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <header className="flex h-16 shrink-0 items-center justify-between border-b px-4">
        <div className="flex items-center gap-2">
          <Trash className="w-4 h-4 text-muted-foreground" />
          <h1 className="font-semibold">Trash</h1>
        </div>
        <button
          onClick={handleEmptyTrash}
          disabled={trashedFiles.length === 0}
          className="text-xs font-medium text-destructive hover:text-destructive/80 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Empty Trash
        </button>
      </header>

      <div className="flex flex-col w-full p-4">
        {loading ? (
          <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
            <Loader2 className="w-6 h-6 animate-spin opacity-40" />
            <span className="text-sm">Loading trash...</span>
          </div>
        ) : trashedFiles.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
            <Trash className="w-10 h-10 opacity-20" />
            <span className="text-sm">Trash is empty.</span>
          </div>
        ) : (
          <>
            {/* Column header */}
            <div className="grid grid-cols-[minmax(0,1fr)_80px] items-center px-3 py-2 border-b border-border/60">
              <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Name</span>
              <span />
            </div>

            <div className="flex flex-col">
              {trashedFiles.map((f) => (
                <div
                  key={f.nodeId}
                  className="group grid grid-cols-[minmax(0,1fr)_80px] items-center px-3 py-1.5 rounded-md transition-colors hover:bg-accent/60"
                >
                  {/* Name + location */}
                  <div className="flex items-center gap-2 min-w-0">
                    {f.type === "FOLDER"
                      ? <FolderIcon className="w-4 h-4 shrink-0 text-muted-foreground/50" />
                      : <FileIcon className="w-4 h-4 shrink-0 text-muted-foreground/50" />
                    }
                    <div className="min-w-0">
                      {f.decrypted ? (
                        <>
                          <span className="text-sm line-through text-muted-foreground block truncate">
                            {f.plaintextName}
                          </span>
                          <span className="text-xs text-muted-foreground/60 block truncate">
                            {f.locationPath}
                          </span>
                        </>
                      ) : (
                        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                          <Loader2 className="w-3 h-3 animate-spin" />
                          Decrypting...
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Restore button — visible on hover */}
                  <div className="flex justify-end opacity-0 group-hover:opacity-100 transition-opacity">
                    <button
                      onClick={() => handleRestore(f)}
                      disabled={!f.decrypted}
                      className="flex items-center gap-1.5 text-xs font-medium text-emerald-600 hover:text-emerald-700 dark:text-emerald-400 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    >
                      <RefreshCw className="w-3 h-3" />
                      Restore
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
