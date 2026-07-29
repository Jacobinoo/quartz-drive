"use client"

import { Button } from "@/components/ui/button"
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
    FileUpIcon, FolderPlusIcon, FolderUpIcon, Plus,
} from "lucide-react"
import { useState } from "react"
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useUploadStore } from "@/hooks/use-upload-store";
import { openFilePicker } from "@/lib/utils/file-picker";
import {getSodium} from "@/lib/crypto/sodium";
import {initFileUpload} from "@/crypto/upload";
import { getAccessToken} from "@/lib/authStore";
import { useDriveStore } from "@/lib/driveStore";
import { createEncryptedFolderPayload } from "@/crypto/folder";
import { customFetch } from "@/lib/api";

export function NewDriveItemButton(){
  const addToQueue = useUploadStore(s => s.addToQueue);
  const [isFolderDialogOpen, setIsFolderDialogOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState("");

    const handleUpload = async () => {

        console.log("opening file picker")

        const files = await openFilePicker({ multiple: true });

      console.log("file picker closed")

      const currentFolder = useDriveStore.getState().getCurrentFolder();
      if (!currentFolder) throw new Error("Drive keys not initialized");


        for (const file of files) {
          console.log(file.name, file.size)

          const nodeId = crypto.randomUUID();
          const parentNodeId = currentFolder.nodeId;
          const chunks = Math.ceil(file.size / (4*1024*1024)) || 1;
          const urls = await initFileUpload(nodeId, chunks);

          console.log(`file "${file.name}" (size: ${file.size}) will be split by ${chunks} chunks, got ${urls.length} presigned urls`);

          // 2. Przygotowanie klucza
          const sodium = await getSodium();
          const fileKey = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES);
          console.log(`file key: ${sodium.to_base64(fileKey)}`);

          // 3. Encrypt the Filename (Symmetric)
          const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
          const encryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
              sodium.from_string(file.name),
              null, // No AD needed for name right now
              null,
              nameNonce,
              currentFolder.privateKey // Using account key as temporary symmetric folder key
          );

          // 4. Wrap the fileKey (Asymmetric Box Seal)
          // Only someone with the accountPrivKey can open this box to retrieve the fileKey!
          const encryptedNodePassphrase = sodium.crypto_box_seal(fileKey, currentFolder.publicKey);

            // 3. Dodanie do reaktywnego Store'a
            console.log("will be adding to queue")
          addToQueue({
            file,
            nodeId,
            fileKey,
            presignedUrls: urls,
            parentNodeId,
            encryptedName: sodium.to_base64(encryptedName),
            nameNonce: sodium.to_base64(nameNonce),
            encryptedNodePassphrase: sodium.to_base64(encryptedNodePassphrase)
          });
        }
    };
    const handleCreateFolder = async () => {
        if (!newFolderName.trim()) return;

        try {
            const folderName = newFolderName.trim();
            setIsFolderDialogOpen(false);
            setNewFolderName("");
            const currentFolder = useDriveStore.getState().getCurrentFolder();
            if (!currentFolder) throw new Error("Drive keys not initialized");


            const token = getAccessToken(); // Replace with however you get your token
            if (!token) throw new Error("No token");
            // 1. Split the token and decode the middle part (the payload)
            const payloadBase64 = token.split('.')[1];
            const decodedPayload = JSON.parse(atob(payloadBase64));

            // For MVP, we'll extract the author ID from the token (like you did for email earlier!)
            // Or you can hardcode a UUID if you haven't wired that up yet.
            const authorId = decodedPayload.sub; // Replace with your real User UUID!

            // Generate the massive crypto payload
            const payload = await createEncryptedFolderPayload(
                folderName,
                currentFolder.nodeId, // Parent is currently the Root
                currentFolder.publicKey,
                currentFolder.privateKey,
                authorId
            );

            // Send it to the Go backend
            const res = await customFetch("https://localhost:3100/v1/files/folder", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload)
            });

            if (res.ok) {
                console.log("✅ Folder created successfully!");
                // Trigger a refresh of your file list here!
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
                    <Plus className="w-5 h-5 mr-1"/>
                    New
                </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent className="w-56" >
                <DropdownMenuItem onClick={handleUpload} className="cursor-pointer">
                    <FileUpIcon className="mr-2 h-4 w-4" />
                    Upload File
                </DropdownMenuItem>
                <DropdownMenuItem disabled>
                    <FolderUpIcon className="mr-2 h-4 w-4" />
                    Upload Folder
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => setIsFolderDialogOpen(true)} className="cursor-pointer">
                    <FolderPlusIcon className="mr-2 h-4 w-4" />
                    New Folder
                </DropdownMenuItem>
            </DropdownMenuContent>
        </DropdownMenu>

        <Dialog open={isFolderDialogOpen} onOpenChange={setIsFolderDialogOpen}>
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
                            onChange={(e) => setNewFolderName(e.target.value)}
                            placeholder="My Secure Folder"
                            className="col-span-3"
                            autoFocus
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') handleCreateFolder();
                            }}
                        />
                    </div>
                </div>
                <DialogFooter>
                    <Button type="button" variant="secondary" onClick={() => setIsFolderDialogOpen(false)}>
                        Cancel
                    </Button>
                    <Button type="submit" onClick={handleCreateFolder}>Create Folder</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
        </>
    )
}
