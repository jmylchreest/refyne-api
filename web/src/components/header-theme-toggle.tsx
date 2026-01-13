'use client';

import * as React from 'react';
import { Moon, Sun, Monitor } from 'lucide-react';
import { useTheme } from 'next-themes';

export function HeaderThemeToggle() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = React.useState(false);

  React.useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted) {
    return (
      <div className="flex items-center gap-0.5 rounded-full bg-secondary px-1.5 py-1">
        <div className="h-5 w-5" />
        <div className="h-5 w-5" />
        <div className="h-5 w-5" />
      </div>
    );
  }

  const options = [
    { value: 'light', icon: Sun, label: 'Light mode' },
    { value: 'dark', icon: Moon, label: 'Dark mode' },
    { value: 'system', icon: Monitor, label: 'System mode' },
  ] as const;

  return (
    <div className="flex items-center gap-0.5 rounded-full bg-secondary px-1.5 py-1">
      {options.map(({ value, icon: Icon, label }) => {
        const isActive = theme === value;
        return (
          <button
            key={value}
            onClick={() => setTheme(value)}
            className={`flex h-5 w-5 items-center justify-center rounded-full transition-all cursor-pointer ${
              isActive
                ? 'text-foreground'
                : 'text-foreground/30 hover:text-foreground/50'
            }`}
            aria-label={label}
            aria-pressed={isActive}
          >
            <Icon className="h-3 w-3" />
          </button>
        );
      })}
    </div>
  );
}
