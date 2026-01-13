'use client';

import { useEffect } from 'react';
import { useAuth } from '@clerk/nextjs';
import { setTokenGetter } from './api';

export function ApiProvider({ children }: { children: React.ReactNode }) {
  const { getToken } = useAuth();

  useEffect(() => {
    // Set up the token getter for the API client
    setTokenGetter(getToken);
  }, [getToken]);

  return <>{children}</>;
}
