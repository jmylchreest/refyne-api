'use client';

import { useState, useCallback, useRef } from 'react';
import { getJobResults } from '@/lib/api';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

// Crawl progress tracking - URLs with status
export interface CrawlProgressUrl {
  url: string;
  timestamp: Date;
  error?: string;
  errorCategory?: string;
  errorDetails?: string; // Full error details (BYOK users only)
}

export interface CrawlProgress {
  extracted: number;
  urlsQueued: number;
  maxPages: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
}

interface UseCrawlStreamOptions {
  onResult?: (url: CrawlProgressUrl) => void;
  onComplete?: (success: boolean, pageCount: number) => void;
  onError?: (error: string) => void;
}

export function useCrawlStream(options: UseCrawlStreamOptions = {}) {
  const [crawlUrls, setCrawlUrls] = useState<CrawlProgressUrl[]>([]);
  const [crawlFinalResult, setCrawlFinalResult] = useState<Record<string, unknown> | null>(null);
  const [crawlProgress, setCrawlProgress] = useState<CrawlProgress>({
    extracted: 0,
    urlsQueued: 0,
    maxPages: 0,
    status: 'pending',
  });
  const [isCrawling, setIsCrawling] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);

  const startStream = useCallback(async (
    jobId: string,
    maxPages: number,
    getToken: () => Promise<string | null>
  ) => {
    // Reset state
    setCrawlUrls([]);
    setCrawlFinalResult(null);
    setIsCrawling(true);
    setCrawlProgress({
      extracted: 0,
      urlsQueued: 0,
      maxPages,
      status: 'running',
    });

    // Create abort controller for cleanup
    abortControllerRef.current = new AbortController();

    try {
      const token = await getToken();
      const streamResponse = await fetch(
        `${API_BASE_URL}/api/v1/jobs/${jobId}/stream`,
        {
          headers: {
            'Authorization': `Bearer ${token}`,
            'Accept': 'text/event-stream',
          },
          signal: abortControllerRef.current.signal,
        }
      );

      if (!streamResponse.ok || !streamResponse.body) {
        setIsCrawling(false);
        setCrawlProgress(prev => ({ ...prev, status: 'failed' }));
        options.onError?.('Failed to connect to crawl stream');
        return;
      }

      const reader = streamResponse.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let currentEvent: string | null = null;
        for (let i = 0; i < lines.length; i++) {
          const line = lines[i];

          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim();
          } else if (line.startsWith('data: ') && currentEvent) {
            const dataStr = line.slice(6);
            try {
              const data = JSON.parse(dataStr);

              if (currentEvent === 'result') {
                const urlEntry: CrawlProgressUrl = {
                  url: data.url,
                  timestamp: new Date(),
                  error: data.error_message,
                  errorCategory: data.error_category,
                  errorDetails: data.error_details,
                };
                setCrawlUrls(prev => [...prev, urlEntry]);
                setCrawlProgress(prev => ({
                  ...prev,
                  extracted: prev.extracted + 1,
                }));
                options.onResult?.(urlEntry);
              } else if (currentEvent === 'status') {
                setCrawlProgress(prev => ({
                  ...prev,
                  extracted: data.page_count || prev.extracted,
                  urlsQueued: data.urls_queued || prev.urlsQueued,
                }));
              } else if (currentEvent === 'complete') {
                setIsCrawling(false);
                const isSuccess = data.status === 'completed';
                setCrawlProgress(prev => ({
                  ...prev,
                  status: isSuccess ? 'completed' : 'failed',
                  extracted: data.page_count || prev.extracted,
                  urlsQueued: data.urls_queued || prev.urlsQueued,
                }));

                if (isSuccess) {
                  // Fetch merged results from backend
                  try {
                    const resultsResponse = await getJobResults(jobId, true);
                    if (resultsResponse.merged) {
                      setCrawlFinalResult(resultsResponse.merged);
                    }
                  } catch (fetchErr) {
                    console.error('Failed to fetch merged results:', fetchErr);
                  }
                }

                options.onComplete?.(isSuccess, data.page_count || 0);
                return;
              }
            } catch {
              // Skip malformed JSON
            }
            currentEvent = null;
          } else if (line === '') {
            currentEvent = null;
          }
        }
      }
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') {
        // Stream was cancelled
        return;
      }
      console.error('Stream error:', err);
      options.onError?.('Connection to crawl stream lost');
    } finally {
      setIsCrawling(false);
    }
  }, [options]);

  const stopStream = useCallback(() => {
    abortControllerRef.current?.abort();
    setIsCrawling(false);
  }, []);

  const reset = useCallback(() => {
    setCrawlUrls([]);
    setCrawlFinalResult(null);
    setCrawlProgress({
      extracted: 0,
      urlsQueued: 0,
      maxPages: 0,
      status: 'pending',
    });
    setIsCrawling(false);
  }, []);

  return {
    crawlUrls,
    crawlFinalResult,
    crawlProgress,
    isCrawling,
    startStream,
    stopStream,
    reset,
  };
}
