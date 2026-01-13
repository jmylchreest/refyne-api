'use client';

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';

export type ProgressStage = {
  id: string;
  label: string;
  icon?: 'connecting' | 'reading' | 'analyzing' | 'thinking' | 'generating' | 'complete' | 'error';
};

interface ProgressStagesProps {
  stages: ProgressStage[];
  currentStage: string;
  isComplete?: boolean;
  isError?: boolean;
  className?: string;
}

export function ProgressStages({
  stages,
  currentStage,
  isComplete = false,
  isError = false,
  className,
}: ProgressStagesProps) {
  const currentIndex = stages.findIndex(s => s.id === currentStage);

  return (
    <div className={cn('flex flex-col gap-3', className)}>
      {stages.map((stage, index) => {
        const isActive = stage.id === currentStage && !isComplete;
        const isPast = index < currentIndex || isComplete;
        const isFuture = index > currentIndex && !isComplete;

        return (
          <div
            key={stage.id}
            className={cn(
              'flex items-center gap-3 transition-all duration-300',
              isActive && 'text-indigo-600 dark:text-indigo-400',
              isPast && 'text-emerald-600 dark:text-emerald-400',
              isFuture && 'text-zinc-400 dark:text-zinc-600',
              isError && isActive && 'text-red-600 dark:text-red-400'
            )}
          >
            <div className="relative flex-shrink-0">
              <StageIcon
                type={stage.icon || 'analyzing'}
                isActive={isActive}
                isPast={isPast}
                isError={isError && isActive}
              />
            </div>
            <span
              className={cn(
                'text-sm font-medium transition-opacity duration-300',
                isFuture && 'opacity-50'
              )}
            >
              {stage.label}
            </span>
          </div>
        );
      })}
    </div>
  );
}

interface StageIconProps {
  type: ProgressStage['icon'];
  isActive: boolean;
  isPast: boolean;
  isError?: boolean;
}

function StageIcon({ type, isActive, isPast, isError }: StageIconProps) {
  if (isPast) {
    return (
      <div className="h-6 w-6 rounded-full bg-emerald-100 dark:bg-emerald-900/40 flex items-center justify-center">
        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
        </svg>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="h-6 w-6 rounded-full bg-red-100 dark:bg-red-900/40 flex items-center justify-center">
        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </div>
    );
  }

  const iconWrapperClass = cn(
    'h-6 w-6 rounded-full flex items-center justify-center',
    isActive
      ? 'bg-indigo-100 dark:bg-indigo-900/40'
      : 'bg-zinc-100 dark:bg-zinc-800'
  );

  switch (type) {
    case 'connecting':
      return (
        <div className={iconWrapperClass}>
          {isActive ? (
            <ConnectingAnimation />
          ) : (
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m13.35-.622l1.757-1.757a4.5 4.5 0 00-6.364-6.364l-4.5 4.5a4.5 4.5 0 001.242 7.244" />
            </svg>
          )}
        </div>
      );
    case 'reading':
      return (
        <div className={iconWrapperClass}>
          {isActive ? (
            <ReadingAnimation />
          ) : (
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
            </svg>
          )}
        </div>
      );
    case 'analyzing':
      return (
        <div className={iconWrapperClass}>
          {isActive ? (
            <AnalyzingAnimation />
          ) : (
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
            </svg>
          )}
        </div>
      );
    case 'thinking':
      return (
        <div className={iconWrapperClass}>
          {isActive ? (
            <ThinkingAnimation />
          ) : (
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 00-2.456 2.456zM16.894 20.567L16.5 21.75l-.394-1.183a2.25 2.25 0 00-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 001.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 001.423 1.423l1.183.394-1.183.394a2.25 2.25 0 00-1.423 1.423z" />
            </svg>
          )}
        </div>
      );
    case 'generating':
      return (
        <div className={iconWrapperClass}>
          {isActive ? (
            <GeneratingAnimation />
          ) : (
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3l-4.5 16.5" />
            </svg>
          )}
        </div>
      );
    case 'complete':
      return (
        <div className="h-6 w-6 rounded-full bg-emerald-100 dark:bg-emerald-900/40 flex items-center justify-center">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
        </div>
      );
    default:
      return (
        <div className={iconWrapperClass}>
          <div className="h-2 w-2 rounded-full bg-current" />
        </div>
      );
  }
}

// Animated icons for active states
function ConnectingAnimation() {
  return (
    <div className="relative h-4 w-4">
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="h-1.5 w-1.5 rounded-full bg-current animate-ping" />
      </div>
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="h-2 w-2 rounded-full border-2 border-current border-t-transparent animate-spin" />
      </div>
    </div>
  );
}

function ReadingAnimation() {
  return (
    <div className="relative h-4 w-4 overflow-hidden">
      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
      </svg>
      <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-current animate-pulse origin-left" />
    </div>
  );
}

function AnalyzingAnimation() {
  return (
    <svg className="h-4 w-4 animate-pulse" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
    </svg>
  );
}

function ThinkingAnimation() {
  return (
    <div className="relative h-4 w-4">
      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
      </svg>
      <div className="absolute -top-0.5 -right-0.5 h-1.5 w-1.5 rounded-full bg-current animate-ping" />
    </div>
  );
}

function GeneratingAnimation() {
  return (
    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3l-4.5 16.5"
        className="animate-pulse"
      />
    </svg>
  );
}

// Circular progress indicator for modal use
interface CircularProgressProps {
  stages: ProgressStage[];
  currentStage: string;
  isComplete?: boolean;
  isError?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

export function CircularProgress({
  stages,
  currentStage,
  isComplete = false,
  isError = false,
  size = 'md',
}: CircularProgressProps) {
  const [dots, setDots] = useState('');
  const currentIndex = stages.findIndex(s => s.id === currentStage);
  const current = stages[currentIndex];

  // Animate dots for text
  useEffect(() => {
    if (isComplete || isError) return;
    const interval = setInterval(() => {
      setDots(prev => (prev.length >= 3 ? '' : prev + '.'));
    }, 400);
    return () => clearInterval(interval);
  }, [isComplete, isError]);

  const sizeClasses = {
    sm: { wrapper: 'h-16 w-16', ring: 'h-14 w-14', icon: 'h-6 w-6' },
    md: { wrapper: 'h-24 w-24', ring: 'h-20 w-20', icon: 'h-8 w-8' },
    lg: { wrapper: 'h-32 w-32', ring: 'h-28 w-28', icon: 'h-10 w-10' },
  };

  const progress = isComplete ? 100 : ((currentIndex + 1) / stages.length) * 100;
  const circumference = 2 * Math.PI * 45; // radius of 45
  const strokeDashoffset = circumference - (progress / 100) * circumference;

  return (
    <div className="flex flex-col items-center gap-4">
      <div className={cn('relative', sizeClasses[size].wrapper)}>
        {/* Background ring */}
        <svg className="absolute inset-0 -rotate-90" viewBox="0 0 100 100">
          <circle
            cx="50"
            cy="50"
            r="45"
            fill="none"
            stroke="currentColor"
            strokeWidth="4"
            className="text-zinc-200 dark:text-zinc-800"
          />
          <circle
            cx="50"
            cy="50"
            r="45"
            fill="none"
            stroke="currentColor"
            strokeWidth="4"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={strokeDashoffset}
            className={cn(
              'transition-all duration-500',
              isError
                ? 'text-red-500'
                : isComplete
                ? 'text-emerald-500'
                : 'text-indigo-500'
            )}
          />
        </svg>

        {/* Center icon */}
        <div className="absolute inset-0 flex items-center justify-center">
          <div
            className={cn(
              'rounded-full flex items-center justify-center',
              sizeClasses[size].ring,
              isError
                ? 'bg-red-50 dark:bg-red-950/30 text-red-600 dark:text-red-400'
                : isComplete
                ? 'bg-emerald-50 dark:bg-emerald-950/30 text-emerald-600 dark:text-emerald-400'
                : 'bg-indigo-50 dark:bg-indigo-950/30 text-indigo-600 dark:text-indigo-400'
            )}
          >
            {isComplete ? (
              <svg className={sizeClasses[size].icon} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
              </svg>
            ) : isError ? (
              <svg className={sizeClasses[size].icon} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
              </svg>
            ) : (
              <div className={cn(sizeClasses[size].icon, 'relative')}>
                <StageIcon type={current?.icon || 'analyzing'} isActive={true} isPast={false} />
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Stage label */}
      <div className="text-center">
        <p
          className={cn(
            'text-sm font-medium',
            isError
              ? 'text-red-600 dark:text-red-400'
              : isComplete
              ? 'text-emerald-600 dark:text-emerald-400'
              : 'text-zinc-900 dark:text-zinc-100'
          )}
        >
          {isComplete ? 'Complete' : isError ? 'Error' : `${current?.label}${dots}`}
        </p>
        {!isComplete && !isError && (
          <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
            Step {currentIndex + 1} of {stages.length}
          </p>
        )}
      </div>
    </div>
  );
}
