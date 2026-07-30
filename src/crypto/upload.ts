import { customFetch } from "@/lib/api";

export async function initFileUpload(
    nodeId: string,
    totalChunks: number
): Promise<Array<string>> {
    const res = await customFetch(`https://localhost:3100/v1/files/upload`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({nodeId: nodeId, totalChunks: totalChunks}),
        credentials: "include",
    });

    const uploadRes = await res.json();
    if(uploadRes.presignedUrls && uploadRes.presignedUrls instanceof Array) {
        return uploadRes.presignedUrls;
    }
    return [];
}

export async function finishFileUpload(payload: {
    nodeId: string;
    parentNodeId: string;
    sizeBytes: number;
    totalChunks: number;
    encryptedName: string;
    nameNonce: string;
    encryptedNodePassphrase: string;
    signedEncryptedNodePassphrase: string;
    nodePublicKey?: string;
    wrappedNodeKey?: string;
    nodePrivNonce?: string;
    chunkNonces: string[];
    chunkSizes: number[];

    encryptedMetadata: string;
     metadataNonce: string;
}): Promise<any> {
    const res = await customFetch(`https://localhost:3100/v1/files/finish`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({
            nodeId: payload.nodeId,
            parentNodeId: payload.parentNodeId,
            sizeBytes: payload.sizeBytes,
            totalChunks: payload.totalChunks,
            encryptedName: payload.encryptedName,
            nameNonce: payload.nameNonce,
            encryptedNodePassphrase: payload.encryptedNodePassphrase,
            signedEncryptedNodePassphrase: payload.signedEncryptedNodePassphrase || "",
            nodePublicKey: payload.nodePublicKey || "",
            wrappedNodeKey: payload.wrappedNodeKey || "",
            nodePrivNonce: payload.nodePrivNonce || "",
            chunkNonces: payload.chunkNonces,
            chunkSizes: payload.chunkSizes,

            encryptedMetadata: payload.encryptedMetadata,
            metadataNonce: payload.metadataNonce,
        }),
        credentials: "include",
    });

    if (!res.ok) {
        const errText = await res.text();
        throw new Error(`Failed to finish file upload: ${errText}`);
    }

    return await res.json();
}
