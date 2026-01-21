'use client';

import { useEffect, useState, useRef } from 'react';
import { cn } from '@/lib/utils';

export type AvatarStage = {
  id: string;
  label: string;
  frames: string[]; // Array of 8-bit pixel art frames (as data URIs or SVG strings)
};

// Default stages with built-in pixel art animations
export const defaultStages: AvatarStage[] = [
  { id: 'connecting', label: 'Connecting...', frames: [] },
  { id: 'reading', label: 'Reading page...', frames: [] },
  { id: 'analyzing', label: 'Analyzing content...', frames: [] },
  { id: 'thinking', label: 'Thinking...', frames: [] },
  { id: 'generating', label: 'Generating schema...', frames: [] },
];

interface ProgressAvatarProps {
  stages?: AvatarStage[];
  currentStage: string;
  isComplete?: boolean;
  isError?: boolean;
  errorMessage?: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export function ProgressAvatar({
  stages = defaultStages,
  currentStage,
  isComplete = false,
  isError = false,
  errorMessage,
  size = 'md',
  className,
}: ProgressAvatarProps) {
  const [frame, setFrame] = useState(0);
  const currentIndex = stages.findIndex(s => s.id === currentStage);
  const current = stages[currentIndex];

  // Animate spinner (braille pattern)
  const spinnerFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];
  const [spinnerIndex, setSpinnerIndex] = useState(0);

  useEffect(() => {
    if (isComplete || isError) return;
    const interval = setInterval(() => {
      setSpinnerIndex(prev => (prev + 1) % spinnerFrames.length);
    }, 80);
    return () => clearInterval(interval);
  }, [isComplete, isError, spinnerFrames.length]);

  // Animate frame cycling for pixel art (continues for complete state)
  useEffect(() => {
    if (isError) return;
    const interval = setInterval(() => {
      setFrame(prev => (prev + 1) % 4);
    }, 250);
    return () => clearInterval(interval);
  }, [isError]);

  const sizeClasses = {
    sm: { wrapper: 'w-48', avatar: 'h-24 w-24', bubble: 'text-xs min-w-44' },
    md: { wrapper: 'w-64', avatar: 'h-36 w-36', bubble: 'text-sm min-w-56' },
    lg: { wrapper: 'w-80', avatar: 'h-48 w-48', bubble: 'text-base min-w-72' },
  };

  const getMessage = () => {
    if (isError) return errorMessage || 'Something went wrong!';
    if (isComplete) return 'All done!';
    return current?.label || 'Working...';
  };

  return (
    <div className={cn('flex flex-col items-center gap-3', sizeClasses[size].wrapper, className)}>
      {/* Avatar in dark container */}
      <div
        className={cn(
          'rounded-lg overflow-hidden',
          'bg-zinc-900 dark:bg-zinc-950',
          'border-2 border-zinc-700 dark:border-zinc-600',
          sizeClasses[size].avatar
        )}
      >
        <div className="relative h-full w-full flex items-center justify-center bg-gradient-to-b from-zinc-800 to-zinc-900">
          <PixelAvatar
            stage={currentStage}
            frame={frame}
            isComplete={isComplete}
            isError={isError}
            size={size}
          />

          {/* Scanline overlay for retro effect */}
          <div
            className="absolute inset-0 pointer-events-none opacity-10"
            style={{
              backgroundImage: 'repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(0,0,0,0.3) 2px, rgba(0,0,0,0.3) 4px)',
            }}
          />
        </div>
      </div>

      {/* Chat bubble - dungeon game style */}
      <div className="relative">
        {/* Bubble pointer */}
        <div
          className={cn(
            'absolute -top-1.5 left-1/2 -translate-x-1/2 w-3 h-3 rotate-45',
            'bg-zinc-100 dark:bg-zinc-800',
            'border-l border-t border-zinc-300 dark:border-zinc-600'
          )}
        />

        {/* Bubble body */}
        <div
          className={cn(
            'relative px-5 py-3 rounded-lg',
            'bg-zinc-100 dark:bg-zinc-800',
            'border-2 border-zinc-300 dark:border-zinc-600',
            'shadow-[2px_2px_0_rgba(0,0,0,0.1)]',
            sizeClasses[size].bubble
          )}
        >
          <p
            className={cn(
              'text-center font-medium whitespace-nowrap',
              'font-[family-name:var(--font-code)]',
              isError && 'text-red-600 dark:text-red-400',
              isComplete && 'text-emerald-600 dark:text-emerald-400',
              !isError && !isComplete && 'text-zinc-800 dark:text-zinc-200'
            )}
          >
            {!isComplete && !isError && (
              <span className="inline-block w-4 text-indigo-500">{spinnerFrames[spinnerIndex]}</span>
            )}
            {' '}{getMessage()}
          </p>
        </div>
      </div>
    </div>
  );
}

interface PixelAvatarProps {
  stage: string;
  frame: number;
  isComplete: boolean;
  isError: boolean;
  size: 'sm' | 'md' | 'lg';
}

function PixelAvatar({ stage, frame, isComplete, isError, size }: PixelAvatarProps) {
  const scale = size === 'sm' ? 1.2 : size === 'lg' ? 2.2 : 1.6;

  // Get the current animation based on stage
  const getAnimation = () => {
    if (isComplete) return <CompleteAnimation frame={frame} />;
    if (isError) return <ErrorAnimation frame={frame} />;

    switch (stage) {
      case 'connecting':
        return <ConnectingPixelAnimation frame={frame} />;
      case 'reading':
        return <ReadingPixelAnimation frame={frame} />;
      case 'analyzing':
        return <AnalyzingPixelAnimation frame={frame} />;
      case 'thinking':
        return <ThinkingPixelAnimation frame={frame} />;
      case 'generating':
        return <GeneratingPixelAnimation frame={frame} />;
      default:
        return <IdleAnimation frame={frame} />;
    }
  };

  return (
    <div
      className="relative flex items-center justify-center"
      style={{ transform: `scale(${scale})` }}
    >
      {getAnimation()}
    </div>
  );
}

// Pixel art animations using CSS grid for 8-bit look
function ConnectingPixelAnimation({ frame }: { frame: number }) {
  return (
    <div className="relative">
      {/* Robot head */}
      <div className="w-12 h-12 relative">
        {/* Head shape */}
        <div className="absolute inset-1 bg-indigo-400 rounded" />
        {/* Eyes */}
        <div className={cn(
          'absolute top-3 left-2 w-2 h-2 bg-white rounded-sm',
          frame % 2 === 0 && 'animate-pulse'
        )} />
        <div className={cn(
          'absolute top-3 right-2 w-2 h-2 bg-white rounded-sm',
          frame % 2 === 0 && 'animate-pulse'
        )} />
        {/* Antenna */}
        <div className="absolute -top-2 left-1/2 -translate-x-1/2 w-1 h-3 bg-indigo-300" />
        <div className={cn(
          'absolute -top-3 left-1/2 -translate-x-1/2 w-2 h-2 rounded-full transition-opacity duration-150',
          frame % 2 === 0 ? 'bg-indigo-200' : 'bg-indigo-400'
        )} />
      </div>
      {/* Connection monitor/screen */}
      <div className="absolute -right-4 top-1/2 -translate-y-1/2 w-6 h-5 bg-zinc-800 rounded-sm border border-zinc-600">
        {/* Screen content - connection status */}
        <div className="absolute inset-0.5 bg-zinc-900 rounded-sm overflow-hidden flex items-center justify-center">
          {/* Animated connection dots */}
          <div className="flex gap-0.5">
            {[0, 1, 2].map(i => (
              <div
                key={i}
                className={cn(
                  'w-1 h-1 rounded-full transition-all duration-150',
                  (frame + i) % 4 <= i ? 'bg-indigo-500' : 'bg-indigo-300/30'
                )}
              />
            ))}
          </div>
        </div>
        {/* Screen glow effect */}
        <div className={cn(
          'absolute inset-0 rounded-sm transition-opacity duration-150',
          frame % 2 === 0 ? 'bg-indigo-400/20' : 'bg-transparent'
        )} />
      </div>
    </div>
  );
}

function ReadingPixelAnimation({ frame }: { frame: number }) {
  return (
    <div className="relative">
      {/* Robot with document */}
      <div className="w-12 h-12 relative">
        {/* Head */}
        <div className="absolute inset-1 bg-cyan-400 rounded" />
        {/* Eyes - reading left to right */}
        <div className={cn(
          'absolute top-3 w-2 h-2 bg-white rounded-sm transition-all duration-200',
          frame === 0 && 'left-2',
          frame === 1 && 'left-3',
          frame === 2 && 'left-4',
          frame === 3 && 'left-3',
        )} />
        <div className={cn(
          'absolute top-3 w-2 h-2 bg-white rounded-sm transition-all duration-200',
          frame === 0 && 'right-4',
          frame === 1 && 'right-3',
          frame === 2 && 'right-2',
          frame === 3 && 'right-3',
        )} />
        {/* Mouth - reading expression */}
        <div className="absolute bottom-2 left-1/2 -translate-x-1/2 w-4 h-1 bg-cyan-600 rounded" />
      </div>
      {/* Document */}
      <div className="absolute -right-3 top-1/2 -translate-y-1/2 w-5 h-6 bg-white rounded-sm border border-zinc-300">
        <div className={cn(
          'absolute top-1 left-0.5 right-0.5 h-0.5 bg-zinc-400 transition-all duration-200',
          frame === 0 && 'top-1',
          frame === 1 && 'top-2',
          frame === 2 && 'top-3',
          frame === 3 && 'top-4',
        )} />
      </div>
    </div>
  );
}

function AnalyzingPixelAnimation({ frame }: { frame: number }) {
  return (
    <div className="relative">
      {/* Robot with magnifying glass */}
      <div className="w-12 h-12 relative">
        {/* Head */}
        <div className="absolute inset-1 bg-amber-400 rounded" />
        {/* Eyes - focused */}
        <div className="absolute top-3 left-2 w-2 h-2 bg-white rounded-sm" />
        <div className={cn(
          'absolute top-3 right-2 w-3 h-3 border-2 border-white rounded-full',
          frame % 2 === 0 && 'scale-110'
        )} />
        {/* Magnifying handle */}
        <div className="absolute top-5 right-0 w-2 h-1 bg-amber-600 rotate-45" />
      </div>
      {/* Scanning effect */}
      <div className={cn(
        'absolute inset-0 border-2 border-amber-300 rounded-lg opacity-50',
        'animate-ping'
      )} style={{ animationDuration: '1.5s' }} />
    </div>
  );
}

function ThinkingPixelAnimation({ frame }: { frame: number }) {
  const thoughtBubbles = [
    { size: 'w-1 h-1', pos: 'right-0 top-2' },
    { size: 'w-1.5 h-1.5', pos: '-right-1 top-0' },
    { size: 'w-2 h-2', pos: '-right-3 -top-2' },
  ];

  return (
    <div className="relative">
      {/* Robot thinking */}
      <div className="w-12 h-12 relative">
        {/* Head */}
        <div className="absolute inset-1 bg-purple-400 rounded" />
        {/* Eyes - looking up in thought */}
        <div className={cn(
          'absolute left-2 w-2 h-2 bg-white rounded-sm transition-all duration-300',
          frame < 2 ? 'top-2' : 'top-3'
        )} />
        <div className={cn(
          'absolute right-2 w-2 h-2 bg-white rounded-sm transition-all duration-300',
          frame < 2 ? 'top-2' : 'top-3'
        )} />
        {/* Thinking mouth */}
        <div className="absolute bottom-2 left-1/2 -translate-x-1/2 w-2 h-1 bg-purple-600 rounded-full" />
      </div>
      {/* Thought bubbles */}
      {thoughtBubbles.map((bubble, i) => (
        <div
          key={i}
          className={cn(
            'absolute bg-white rounded-full transition-opacity duration-300',
            bubble.size,
            bubble.pos,
            (frame + i) % 4 < 2 ? 'opacity-100' : 'opacity-40'
          )}
        />
      ))}
    </div>
  );
}

function GeneratingPixelAnimation({ frame }: { frame: number }) {
  // Code lines that build up progressively - random widths that fit in the box
  const codeLines = [
    { width: 'w-2.5', delay: 0 },
    { width: 'w-3.5', delay: 1 },
    { width: 'w-2', delay: 2 },
    { width: 'w-3', delay: 3 },
  ];

  return (
    <div className="relative">
      {/* Robot generating */}
      <div className="w-12 h-12 relative">
        {/* Head */}
        <div className="absolute inset-1 bg-emerald-400 rounded" />
        {/* Eyes - focused on work */}
        <div className={cn(
          'absolute top-3 left-2 w-2 h-2 bg-white rounded-sm',
          frame % 2 === 0 && 'h-1.5'
        )} />
        <div className={cn(
          'absolute top-3 right-2 w-2 h-2 bg-white rounded-sm',
          frame % 2 === 0 && 'h-1.5'
        )} />
        {/* Concentrated mouth */}
        <div className="absolute bottom-2 left-1/2 -translate-x-1/2 w-2 h-1 bg-emerald-600 rounded" />
      </div>
      {/* Document being written */}
      <div className="absolute -right-4 top-0 w-6 h-10 bg-white rounded-sm border border-zinc-300 overflow-hidden">
        {/* Opening bracket - smaller */}
        <div className="absolute top-1 left-1 text-[4px] text-emerald-600 font-bold font-mono leading-none">{'{'}</div>
        {/* Code lines building up - positioned between braces */}
        {codeLines.map((line, i) => (
          <div
            key={i}
            className={cn(
              'absolute left-1.5 h-0.5 bg-zinc-400 transition-all duration-150',
              line.width,
              (frame + line.delay) % 4 >= i ? 'opacity-100' : 'opacity-0'
            )}
            style={{ top: `${10 + i * 5}px` }}
          />
        ))}
        {/* Closing bracket - smaller */}
        <div className={cn(
          'absolute bottom-1 left-1 text-[4px] text-emerald-600 font-bold font-mono leading-none transition-opacity duration-150',
          frame === 3 ? 'opacity-100' : 'opacity-30'
        )}>{'}'}</div>
      </div>
    </div>
  );
}

function CompleteAnimation({ frame }: { frame: number }) {
  return (
    <div className="relative">
      <div className="w-12 h-12 relative">
        {/* Head - happy green */}
        <div className="absolute inset-1 bg-emerald-400 rounded" />
        {/* Eyes - happy closed (curved like smiling) */}
        <div className="absolute top-3 left-2 w-2 h-1 border-b-2 border-white rounded-b-full" />
        <div className="absolute top-3 right-2 w-2 h-1 border-b-2 border-white rounded-b-full" />
        {/* Big smile - grows with animation */}
        <div className={cn(
          'absolute bottom-1.5 left-1/2 -translate-x-1/2 bg-emerald-600 rounded-b-full transition-all duration-150',
          frame < 2 ? 'w-4 h-2' : 'w-5 h-2.5'
        )} />
      </div>
      {/* Checkbox with animated tick */}
      <div className="absolute -right-4 top-1/2 -translate-y-1/2 w-6 h-6 bg-white rounded-sm border-2 border-emerald-500">
        {/* Animated checkmark */}
        <svg
          viewBox="0 0 24 24"
          className="absolute inset-0 w-full h-full p-0.5"
          fill="none"
          stroke="currentColor"
          strokeWidth={4}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {/* Short stroke (down-left part of check) */}
          <path
            d="M6 12 L10 16"
            className={cn(
              'text-emerald-500 transition-opacity duration-100',
              frame >= 1 ? 'opacity-100' : 'opacity-0'
            )}
          />
          {/* Long stroke (up-right part of check) */}
          <path
            d="M10 16 L18 8"
            className={cn(
              'text-emerald-500 transition-opacity duration-100',
              frame >= 2 ? 'opacity-100' : 'opacity-0'
            )}
          />
        </svg>
        {/* Success glow */}
        <div className={cn(
          'absolute inset-0 rounded-sm transition-opacity duration-150',
          frame === 3 ? 'bg-emerald-400/30' : 'bg-transparent'
        )} />
      </div>
    </div>
  );
}

function ErrorAnimation({ frame }: { frame: number }) {
  return (
    <div className="relative">
      <div className="w-12 h-12 relative">
        {/* Head - red */}
        <div className="absolute inset-1 bg-red-400 rounded" />
        {/* X eyes */}
        <div className="absolute top-3 left-2 w-2 h-2">
          <div className="absolute inset-0 bg-white rotate-45 rounded-sm" style={{ clipPath: 'polygon(40% 0, 60% 0, 60% 40%, 100% 40%, 100% 60%, 60% 60%, 60% 100%, 40% 100%, 40% 60%, 0 60%, 0 40%, 40% 40%)' }} />
        </div>
        <div className="absolute top-3 right-2 w-2 h-2">
          <div className="absolute inset-0 bg-white rotate-45 rounded-sm" style={{ clipPath: 'polygon(40% 0, 60% 0, 60% 40%, 100% 40%, 100% 60%, 60% 60%, 60% 100%, 40% 100%, 40% 60%, 0 60%, 0 40%, 40% 40%)' }} />
        </div>
        {/* Sad mouth */}
        <div className={cn(
          'absolute bottom-2 left-1/2 -translate-x-1/2 w-4 h-1.5 bg-red-600 rounded-t-full',
          frame % 2 === 0 && 'opacity-80'
        )} />
      </div>
    </div>
  );
}

function IdleAnimation({ frame }: { frame: number }) {
  return (
    <div className="w-12 h-12 relative">
      {/* Head */}
      <div className="absolute inset-1 bg-zinc-400 rounded" />
      {/* Eyes - blinking */}
      <div className={cn(
        'absolute top-3 left-2 w-2 bg-white rounded-sm transition-all duration-100',
        frame === 3 ? 'h-0.5' : 'h-2'
      )} />
      <div className={cn(
        'absolute top-3 right-2 w-2 bg-white rounded-sm transition-all duration-100',
        frame === 3 ? 'h-0.5' : 'h-2'
      )} />
      {/* Neutral mouth */}
      <div className="absolute bottom-2 left-1/2 -translate-x-1/2 w-3 h-0.5 bg-zinc-600 rounded" />
    </div>
  );
}

// Export a dialog-style wrapper for modal use
interface ProgressAvatarDialogProps {
  open: boolean;
  stages?: AvatarStage[];
  currentStage: string;
  isComplete?: boolean;
  isError?: boolean;
  errorMessage?: string;
  onClose?: () => void;
  autoCloseDelay?: number; // Auto-close delay in ms for success state (default: 3000)
}

export function ProgressAvatarDialog({
  open,
  stages = defaultStages,
  currentStage,
  isComplete = false,
  isError = false,
  errorMessage,
  onClose,
  autoCloseDelay = 3000,
}: ProgressAvatarDialogProps) {
  const autoCloseTimerRef = useRef<NodeJS.Timeout | null>(null);

  // Auto-close on success after delay
  useEffect(() => {
    if (isComplete && !isError && onClose && autoCloseDelay > 0) {
      autoCloseTimerRef.current = setTimeout(() => {
        onClose();
      }, autoCloseDelay);
    }

    return () => {
      if (autoCloseTimerRef.current) {
        clearTimeout(autoCloseTimerRef.current);
      }
    };
  }, [isComplete, isError, onClose, autoCloseDelay]);

  if (!open) return null;

  // Handle click to close (for success state, clicking anywhere closes immediately)
  const handleClick = () => {
    if ((isComplete || isError) && onClose) {
      if (autoCloseTimerRef.current) {
        clearTimeout(autoCloseTimerRef.current);
      }
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={handleClick}
      style={{ cursor: isComplete || isError ? 'pointer' : 'default' }}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" />

      {/* Dialog - transparent container */}
      <div className="relative p-6" onClick={(e) => e.stopPropagation()}>
        <div
          onClick={handleClick}
          style={{ cursor: isComplete || isError ? 'pointer' : 'default' }}
        >
          <ProgressAvatar
            stages={stages}
            currentStage={currentStage}
            isComplete={isComplete}
            isError={isError}
            errorMessage={errorMessage}
            size="lg"
          />
        </div>

        {/* Only show button for error state */}
        {isError && onClose && (
          <button
            onClick={onClose}
            className={cn(
              'mt-4 w-full py-2 px-4 rounded font-medium text-sm transition-colors',
              'border-2 shadow-[2px_2px_0_rgba(0,0,0,0.1)]',
              'bg-red-100 dark:bg-red-900/30 border-red-300 dark:border-red-700 text-red-700 dark:text-red-300 hover:bg-red-200 dark:hover:bg-red-900/50'
            )}
          >
            Try Again
          </button>
        )}
      </div>
    </div>
  );
}
