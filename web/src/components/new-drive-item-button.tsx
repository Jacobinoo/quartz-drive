"use client";
import { config } from "@/config/env";

import { Button } from "@/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { FileUpIcon, FolderPlusIcon, FolderUpIcon, Plus } from "lucide-react";
import { useState } from "react";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { openFilePicker } from "@/lib/utils/file-picker";
import { getAccessToken, getAccountSigningPrivateKey } from "@/lib/authStore";
import { createEncryptedFolderPayload } from "@/crypto/folder";
import { customFetch } from "@/lib/api";
import { addSingleSearchItem } from "@/lib/SearchIndexStore";
import { enqueueFileUploads } from "@/lib/upload/enqueue-files";
import { useDriveStore } from "@/lib/driveStore";

export function NewDriveItemButton() {
    const [isFolderDialogOpen, setIsFolderDialogOpen] = useState(false);
    const [newFolderName, setNewFolderName] = useState("");

    const handleUpload = async () => {
        console.log("opening file picker");

        const files = await openFilePicker({ multiple: true });
        if (files.length > 0) await enqueueFileUploads(files);
    };
    const handleCreateFolder = async () => {
        if (!newFolderName.trim()) return;

        try {
            const folderName = newFolderName.trim();
            setIsFolderDialogOpen(false);
            setNewFolderName("");
            const currentFolder = useDriveStore.getState().getCurrentFolder();
            if (!currentFolder) throw new Error("Drive keys not initialized");

            const token = getAccessToken();
            if (!token) throw new Error("No token");

            const payloadBase64 = token.split(".")[1];
            const decodedPayload = JSON.parse(atob(payloadBase64));

            const authorId = decodedPayload.sub;

            const payload = await createEncryptedFolderPayload(
                folderName,
                currentFolder.nodeId,
                currentFolder.publicKey,
                currentFolder.privateKey,
                authorId
            );

            const res = await customFetch(`${config.apiUrl}/v1/files/folder`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload),
            });

            if (res.ok) {
                console.debug("✅ Folder created successfully!");
                const data = await res.json();
                await addSingleSearchItem({
                    id: data.nodeId,
                    name: folderName,
                    type: "FOLDER",
                    sizeBytes: 0,
                    createdAt: new Date().toISOString(),
                });
                useDriveStore.getState().triggerRefresh();
            }
        } catch (err) {
            console.error("Failed to create folder", err);
        }
    };

    return (
        <>
            <DropdownMenu>
                <DropdownMenuTrigger asChild>
                    <Button className="bg-blue-600 hover:bg-blue-700 text-white h-11 mx-2 transition-colors">
                        <Plus className="w-5 h-5 mr-1" />
                        New
                    </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent className="w-56">
                    <DropdownMenuItem
                        onClick={handleUpload}
                        className="cursor-pointer"
                    >
                        <FileUpIcon className="mr-2 h-4 w-4" />
                        Upload File
                    </DropdownMenuItem>
                    <DropdownMenuItem disabled>
                        <FolderUpIcon className="mr-2 h-4 w-4" />
                        Upload Folder
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                        onClick={() => setIsFolderDialogOpen(true)}
                        className="cursor-pointer"
                    >
                        <FolderPlusIcon className="mr-2 h-4 w-4" />
                        New Folder
                    </DropdownMenuItem>
                </DropdownMenuContent>
            </DropdownMenu>

            <Dialog
                open={isFolderDialogOpen}
                onOpenChange={setIsFolderDialogOpen}
            >
                <DialogContent className="sm:max-w-[425px]">
                    <DialogHeader>
                        <DialogTitle>Create New Folder</DialogTitle>
                    </DialogHeader>
                    <div className="grid gap-4 py-4">
                        <div className="grid grid-cols-4 items-center gap-4">
                            <Label htmlFor="name" className="text-right">
                                Name
                            </Label>
                            <Input
                                id="name"
                                value={newFolderName}
                                onChange={(e) =>
                                    setNewFolderName(e.target.value)
                                }
                                placeholder="My Secure Folder"
                                className="col-span-3"
                                autoFocus
                                onKeyDown={(e) => {
                                    if (e.key === "Enter") handleCreateFolder();
                                }}
                            />
                        </div>
                    </div>
                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => setIsFolderDialogOpen(false)}
                        >
                            Cancel
                        </Button>
                        <Button type="submit" onClick={handleCreateFolder}>
                            Create Folder
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </>
    );
}
