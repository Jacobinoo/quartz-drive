import { useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { shareFolderCryptographically } from "@/lib/shareService";

export function ShareModal({ folder, isOpen, onClose }: { folder: any, isOpen: boolean, onClose: () => void }) {
    const [email, setEmail] = useState("");
    const [loading, setLoading] = useState(false);

    const handleShare = async () => {
        if (!email) return;
        setLoading(true);
        try {
            await shareFolderCryptographically(folder, email);
            alert("Folder shared successfully!");
            onClose();
            setEmail("");
        } catch (e: any) {
            console.error(e);
            alert("Failed to share folder: " + e.message);
        } finally {
            setLoading(false);
        }
    };

    if (!folder) return null;

    return (
        <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
            <DialogContent className="sm:max-w-md">
                <DialogHeader>
                    <DialogTitle>Share "{folder.plaintextName}"</DialogTitle>
                </DialogHeader>
                <div className="flex flex-col space-y-4 py-4">
                    <p className="text-sm text-gray-500">
                        Enter the email address of the user you want to share this folder with. Everything inside will be end-to-end encrypted for them.
                    </p>
                    <Input
                        placeholder="jane@example.com"
                        value={email}
                        onChange={(e) => setEmail(e.target.value)}
                        disabled={loading}
                        type="email"
                    />
                </div>
                <DialogFooter>
                    <Button variant="outline" onClick={onClose} disabled={loading}>Cancel</Button>
                    <Button onClick={handleShare} disabled={loading || !email}>
                        {loading ? "Encrypting..." : "Send Invite"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
