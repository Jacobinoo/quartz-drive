import { config } from "@/config/env";
import { customFetch } from "@/lib/api";

export async function fetchFiles(folderId?: string) {
    const url = folderId
        ? `${config.apiUrl}/v1/files?folderId=${folderId}`
        : `${config.apiUrl}/v1/files`;

    const res = await customFetch(url, { method: "GET" });
    if (!res.ok) throw new Error("Failed to fetch files");

    return await res.json();
}

export async function getDownloadUrls(nodeId: string): Promise<string[]> {
    const res = await customFetch(`${config.apiUrl}/v1/files/download?nodeId=${nodeId}`, { method: "GET" });
    if (!res.ok) throw new Error("Failed to get download URLs");

    const data = await res.json();
    return data.presignedUrls;
}
