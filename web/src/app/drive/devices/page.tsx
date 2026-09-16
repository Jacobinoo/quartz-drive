"use client";

import { useEffect, useState } from "react";
import { MonitorSmartphone, Trash2, Loader2, RefreshCw } from "lucide-react";
import { getDevices, revokeDevice, type DeviceResponse } from "@/lib/devices";

export default function DevicesPage() {
    const [devices, setDevices] = useState<DeviceResponse[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        loadDevices();
    }, []);

    async function loadDevices() {
        setLoading(true);
        try {
            const data = await getDevices();
            setDevices(data);
        } catch (err) {
            console.error("Failed to load devices:", err);
        } finally {
            setLoading(false);
        }
    }

    async function handleRevoke(sessionId: string) {
        if (!confirm("Are you sure you want to revoke this session? The device will be immediately logged out.")) {
            return;
        }

        try {
            await revokeDevice(sessionId);
            // Refresh list
            await loadDevices();
        } catch (e) {
            console.error("Revoke failed:", e);
            alert("Failed to revoke session.");
        }
    }

    function formatDate(dateStr: string) {
        const date = new Date(dateStr);
        return date.toLocaleString();
    }

    return (
        <div className="flex flex-col h-full">
            <header className="flex h-16 shrink-0 items-center justify-between border-b px-4">
                <div className="flex items-center gap-2">
                    <MonitorSmartphone className="w-4 h-4 text-muted-foreground" />
                    <h1 className="font-semibold">Active Sessions</h1>
                </div>
                <button
                    onClick={loadDevices}
                    disabled={loading}
                    className="flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
                >
                    <RefreshCw className={`w-3 h-3 ${loading ? "animate-spin" : ""}`} />
                    Refresh
                </button>
            </header>

            <div className="flex flex-col w-full p-4">
                {loading && devices.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
                        <Loader2 className="w-6 h-6 animate-spin opacity-40" />
                        <span className="text-sm">Loading active sessions...</span>
                    </div>
                ) : devices.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
                        <MonitorSmartphone className="w-10 h-10 opacity-20" />
                        <span className="text-sm">No active sessions found.</span>
                    </div>
                ) : (
                    <>
                        <div className="grid grid-cols-[1.5fr_minmax(0,1fr)_80px] items-center px-3 py-2 border-b border-border/60">
                            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Device & Browser</span>
                            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Last Active</span>
                            <span />
                        </div>

                        <div className="flex flex-col">
                            {devices.map((device) => (
                                <div
                                    key={device.sessionId}
                                    className="group grid grid-cols-[1.5fr_minmax(0,1fr)_80px] items-center px-3 py-3 rounded-md transition-colors hover:bg-accent/60 border-b border-border/30 last:border-0"
                                >
                                    <div className="flex items-center gap-3 min-w-0">
                                        <div className="p-2 rounded-full bg-accent text-muted-foreground shrink-0">
                                            <MonitorSmartphone className="w-4 h-4" />
                                        </div>
                                        <div className="min-w-0">
                                            <span className="text-sm font-medium text-foreground block truncate flex items-center gap-2">
                                                {device.deviceName}
                                                {device.isCurrent && (
                                                    <span className="px-1.5 py-0.5 rounded text-[10px] uppercase font-bold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                                                        Current Session
                                                    </span>
                                                )}
                                            </span>
                                            <span className="text-xs text-muted-foreground/60 block truncate" title={device.userAgent}>
                                                {device.userAgent}
                                            </span>
                                        </div>
                                    </div>

                                    <div className="text-sm text-muted-foreground truncate">
                                        {formatDate(device.lastActiveAt)}
                                    </div>

                                    <div className="flex justify-end opacity-0 group-hover:opacity-100 transition-opacity">
                                        {!device.isCurrent && (
                                            <button
                                                onClick={() => handleRevoke(device.sessionId)}
                                                className="flex items-center gap-1.5 text-xs font-medium text-destructive hover:text-destructive/80 transition-colors"
                                                title="Revoke Session"
                                            >
                                                <Trash2 className="w-4 h-4" />
                                                Revoke
                                            </button>
                                        )}
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
