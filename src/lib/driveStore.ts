import { create } from 'zustand';

export interface FolderKey {
    nodeId: string;
    name: string; // Plaintext name
    privateKey: Uint8Array;
    publicKey: Uint8Array;
  hasChildren?: boolean;
  isShared?: boolean;
}

interface DriveState {
  breadcrumbs: FolderKey[];
  setBreadcrumbs: (breadcrumbs: FolderKey[]) => void;
  myDriveRoot: FolderKey | null;
  setMyDriveRoot: (folder: FolderKey) => void;
  draggedItem: any | null;
  refreshTrigger: number;

    // Helper to get the keys for whatever folder you are currently viewing
    getCurrentFolder: () => FolderKey | null;

    // Core actions
    setRootFolder: (folder: FolderKey) => void;
    pushFolder: (folder: FolderKey) => void;
  popFolder: () => void;

  jumpToFolder: (index: number) => void;
  setDraggedItem: (item: any | null) => void;
  triggerRefresh: () => void;
}

export const useDriveStore = create<DriveState>((set, get) => ({
  breadcrumbs: [],
  setBreadcrumbs: (breadcrumbs) => set({ breadcrumbs }),
  myDriveRoot: null,
  setMyDriveRoot: (folder) => set({ myDriveRoot: folder }),
  draggedItem: null,
    refreshTrigger: 0,

    getCurrentFolder: () => {
        const stack = get().breadcrumbs;
        return stack.length > 0 ? stack[stack.length - 1] : null;
    },

    setRootFolder: (folder) => set({ breadcrumbs: [folder] }),
    pushFolder: (folder) => set((state) => ({ breadcrumbs: [...state.breadcrumbs, folder] })),
    popFolder: () => set((state) => ({
        breadcrumbs: state.breadcrumbs.length > 1
            ? state.breadcrumbs.slice(0, -1)
            : state.breadcrumbs
    })),
        jumpToFolder: (index) => set((state) => ({
            breadcrumbs: state.breadcrumbs.slice(0, index + 1)
        })),
  setDraggedItem: (item) => set({ draggedItem: item }),
        triggerRefresh: () => set((state) => ({ refreshTrigger: state.refreshTrigger + 1 })),
}));
