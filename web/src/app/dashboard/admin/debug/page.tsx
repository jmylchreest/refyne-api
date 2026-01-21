'use client';

import { useState } from 'react';
import { ProgressAvatar } from '@/components/ui/progress-avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

const allStages = [
  { id: 'connecting', label: 'Connecting...', frames: [] },
  { id: 'reading', label: 'Reading page...', frames: [] },
  { id: 'analyzing', label: 'Analyzing content...', frames: [] },
  { id: 'thinking', label: 'Thinking...', frames: [] },
  { id: 'generating', label: 'Generating schema...', frames: [] },
];

export default function DebugPage() {
  const [selectedStage, setSelectedStage] = useState<string | null>(null);
  const [showComplete, setShowComplete] = useState(false);
  const [showError, setShowError] = useState(false);

  return (
    <div className="space-y-6">
      <p className="text-zinc-600 dark:text-zinc-400">
        Preview all progress avatar animations and states.
      </p>

      {/* Controls */}
      <Card>
        <CardHeader>
          <CardTitle>Controls</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Button
              variant={selectedStage === null && !showComplete && !showError ? 'default' : 'outline'}
              onClick={() => {
                setSelectedStage(null);
                setShowComplete(false);
                setShowError(false);
              }}
            >
              Show All
            </Button>
            <Button
              variant={showComplete ? 'default' : 'outline'}
              onClick={() => {
                setSelectedStage(null);
                setShowComplete(true);
                setShowError(false);
              }}
            >
              Complete State
            </Button>
            <Button
              variant={showError ? 'default' : 'outline'}
              onClick={() => {
                setSelectedStage(null);
                setShowComplete(false);
                setShowError(true);
              }}
            >
              Error State
            </Button>
          </div>
          <div className="flex flex-wrap gap-2 mt-4">
            {allStages.map((stage) => (
              <Button
                key={stage.id}
                variant={selectedStage === stage.id ? 'default' : 'outline'}
                size="sm"
                onClick={() => {
                  setSelectedStage(stage.id);
                  setShowComplete(false);
                  setShowError(false);
                }}
              >
                {stage.id}
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Single Stage Preview */}
      {(selectedStage || showComplete || showError) && (
        <Card>
          <CardHeader>
            <CardTitle>
              {showComplete ? 'Complete State' : showError ? 'Error State' : `Stage: ${selectedStage}`}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex justify-center py-8 bg-zinc-900/50 rounded-lg">
              <ProgressAvatar
                stages={allStages}
                currentStage={selectedStage || 'connecting'}
                isComplete={showComplete}
                isError={showError}
                errorMessage="Something went wrong!"
                size="lg"
              />
            </div>
          </CardContent>
        </Card>
      )}

      {/* All Stages Grid */}
      {!selectedStage && !showComplete && !showError && (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {allStages.map((stage) => (
            <Card key={stage.id}>
              <CardHeader className="pb-2">
                <CardTitle className="text-lg">{stage.id}</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex justify-center py-6 bg-zinc-900/50 rounded-lg">
                  <ProgressAvatar
                    stages={allStages}
                    currentStage={stage.id}
                    size="md"
                  />
                </div>
              </CardContent>
            </Card>
          ))}

          {/* Complete state */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-lg text-emerald-600">complete</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex justify-center py-6 bg-zinc-900/50 rounded-lg">
                <ProgressAvatar
                  stages={allStages}
                  currentStage="generating"
                  isComplete={true}
                  size="md"
                />
              </div>
            </CardContent>
          </Card>

          {/* Error state */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-lg text-red-600">error</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex justify-center py-6 bg-zinc-900/50 rounded-lg">
                <ProgressAvatar
                  stages={allStages}
                  currentStage="analyzing"
                  isError={true}
                  errorMessage="Something went wrong!"
                  size="md"
                />
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Size Comparison */}
      <Card>
        <CardHeader>
          <CardTitle>Size Comparison</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-end justify-center gap-8 py-6 bg-zinc-900/50 rounded-lg">
            <div className="text-center">
              <ProgressAvatar
                stages={allStages}
                currentStage="thinking"
                size="sm"
              />
              <p className="mt-2 text-xs text-zinc-500">Small</p>
            </div>
            <div className="text-center">
              <ProgressAvatar
                stages={allStages}
                currentStage="thinking"
                size="md"
              />
              <p className="mt-2 text-xs text-zinc-500">Medium</p>
            </div>
            <div className="text-center">
              <ProgressAvatar
                stages={allStages}
                currentStage="thinking"
                size="lg"
              />
              <p className="mt-2 text-xs text-zinc-500">Large</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
