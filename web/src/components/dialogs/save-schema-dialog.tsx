'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

interface SaveSchemaDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (name: string, description: string) => void;
  defaultName?: string;
}

export function SaveSchemaDialog({
  open,
  onOpenChange,
  onSave,
  defaultName = '',
}: SaveSchemaDialogProps) {
  const [name, setName] = useState(defaultName);
  const [description, setDescription] = useState('');

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setName('');
      setDescription('');
    } else if (defaultName) {
      setName(defaultName);
    }
    onOpenChange(newOpen);
  };

  const handleSave = () => {
    if (!name) return;
    onSave(name, description);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Save Schema</DialogTitle>
          <DialogDescription>
            Save this schema to your catalog for future use.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="schema-name">Name</Label>
            <Input
              id="schema-name"
              placeholder="My Product Schema"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="schema-description">Description (optional)</Label>
            <Textarea
              id="schema-description"
              placeholder="Schema for extracting product data..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={!name}>
            Save Schema
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
