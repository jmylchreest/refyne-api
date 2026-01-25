'use client';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

interface OverwriteConfirmationDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  itemName: string;
  itemDetail?: string;
  onOverwrite: () => void;
  onCreateNew: () => void;
}

export function OverwriteConfirmationDialog({
  open,
  onOpenChange,
  title,
  description,
  itemName,
  itemDetail,
  onOverwrite,
  onCreateNew,
}: OverwriteConfirmationDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <div className="rounded-md bg-zinc-100 dark:bg-zinc-800 p-3">
            <p className="text-sm font-medium text-zinc-700 dark:text-zinc-300">
              Existing item:
            </p>
            <p className="text-sm text-zinc-500 dark:text-zinc-400 font-mono mt-1 truncate">
              {itemName}
            </p>
            {itemDetail && (
              <p className="text-xs text-zinc-400 mt-1">{itemDetail}</p>
            )}
          </div>
        </div>
        <DialogFooter className="flex-col sm:flex-row gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="secondary" onClick={onCreateNew}>
            Create New
          </Button>
          <Button onClick={onOverwrite}>Overwrite Existing</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
