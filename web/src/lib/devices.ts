import { config } from "@/config/env";
import { customFetch } from "./api";

export interface DeviceResponse {
    sessionId: string;
    deviceName: string;
    userAgent: string;
    lastActiveAt: string;
    createdAt: string;
    isCurrent: boolean;
}

export async function getDevices(): Promise<DeviceResponse[]> {
    const response = await customFetch(`${config.apiUrl}/v1/devices/list`);
    if (!response.ok) {
        throw new Error("Failed to fetch devices");
    }
    return response.json();
}

export async function revokeDevice(sessionId: string): Promise<void> {
    const response = await customFetch(`${config.apiUrl}/v1/devices/revoke`, {
        method: "DELETE",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({ sessionId }),
    });
    if (!response.ok) {
        throw new Error("Failed to revoke device");
    }
}
