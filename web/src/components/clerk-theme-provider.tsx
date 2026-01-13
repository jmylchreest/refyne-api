'use client';

import { ClerkProvider } from '@clerk/nextjs';
import { dark } from '@clerk/themes';
import { useTheme } from 'next-themes';
import { useEffect, useState } from 'react';

export function ClerkThemeProvider({ children }: { children: React.ReactNode }) {
  const { resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Before mounting, render without theme to avoid hydration mismatch
  if (!mounted) {
    return <ClerkProvider>{children}</ClerkProvider>;
  }

  return (
    <ClerkProvider
      appearance={{
        baseTheme: resolvedTheme === 'dark' ? dark : undefined,
        variables: {
          colorBackground: resolvedTheme === 'dark' ? '#18181b' : undefined, // zinc-900
          colorInputBackground: resolvedTheme === 'dark' ? '#27272a' : undefined, // zinc-800
          colorText: resolvedTheme === 'dark' ? '#f4f4f5' : undefined, // zinc-100
          colorTextSecondary: resolvedTheme === 'dark' ? '#a1a1aa' : undefined, // zinc-400
        },
      }}
    >
      {children}
    </ClerkProvider>
  );
}
