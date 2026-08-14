import { config } from "@/config/env";
import { customFetch } from "@/lib/api";
import { UUID } from "crypto";

export async function initFileUpload(payload: {
  totalChunks: number;
  totalFileSize: number

  parentNodeId: string;
  encryptedName: string;
  nameNonce: string;
  encryptedNodePassphrase: string;
  signedEncryptedNodePassphrase: string;
  nodePublicKey?: string;
  wrappedNodeKey?: string;
  nodePrivNonce?: string;

  encryptedMetadata: string;
  metadataNonce: string;
}): Promise<{
  uploadId: UUID,
  nodeId: UUID,
  expiresAt: number
} | null> {
  const res = await customFetch(`${config.apiUrl}/v1/files/upload/init`, {
    method: "POST",
    headers: {
        "Content-Type": "application/json",
    },
    body: JSON.stringify({
      totalFileSize: payload.totalFileSize,
      totalChunks: payload.totalChunks,
      parentNodeId: payload.parentNodeId,
      encryptedName: payload.encryptedName,
      nameNonce: payload.nameNonce,
      encryptedNodePassphrase: payload.encryptedNodePassphrase,
      signedEncryptedNodePassphrase: payload.signedEncryptedNodePassphrase,
      nodePublicKey: payload.nodePublicKey,
      wrappedNodeKey: payload.wrappedNodeKey,
      nodePrivNonce: payload.nodePrivNonce,

      encryptedMetadata: payload.encryptedMetadata,
       metadataNonce: payload.metadataNonce,
    }),
  })

  const uploadRes = await res.json();
  if (uploadRes.uploadId && uploadRes.nodeId && uploadRes.expiresAt) {
    return {
      uploadId: uploadRes.uploadId,
      nodeId: uploadRes.nodeId,
      expiresAt: uploadRes.expiresAt
    }
  }
  return null
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
    const res = await customFetch(`${config.apiUrl}/v1/files/finish`, {
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
